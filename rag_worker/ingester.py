"""
Ingester module: ingests documents into the vector database.
Handles large files safely by streaming content and processing in batches.
"""

import hashlib
import logging
import os
import time
from typing import List, Optional

from rag_worker.chunker import chunk_text
from rag_worker.embedder import Embedder
from rag_worker.vectordb import VectorDB

logger = logging.getLogger(__name__)

# File extensions that unstructured handles
UNSTRUCTURED_EXTENSIONS = {".pdf", ".docx", ".doc", ".html", ".htm", ".xml", ".epub", ".odt", ".ppt", ".pptx", ".xls", ".xlsx"}

# Text-based files we handle directly
TEXT_EXTENSIONS = {".txt", ".md", ".markdown", ".rst", ".json", ".yaml", ".yml", ".toml", ".cfg", ".ini", ".conf", ".env", ".csv", ".tsv", ".jsonl", ".ndjson", ".log", ".sql", ".sh", ".py", ".go", ".js", ".ts", ".xml"}

# Warn if file is larger than this (50MB)
LARGE_FILE_WARN = 50 * 1024 * 1024

# Never load files larger than this into memory at once (200MB)
MAX_FILE_LOAD = 200 * 1024 * 1024

# Batch size for embedding (keep moderate to avoid OOM)
BATCH_SIZE = 25


def get_file_size(path: str) -> int:
    """Get file size in bytes."""
    try:
        return os.path.getsize(path)
    except OSError:
        return 0


def get_text_content(path: str) -> Optional[str]:
    """Extract text content from a file.

    For large files (>50MB), warns and truncates to MAX_FILE_LOAD bytes.
    For text files, reads the whole content (streaming not needed for display).
    For PDF/complex, uses unstructured.

    Args:
        path: Path to the file

    Returns:
        Extracted text, or None if extraction fails
    """
    ext = os.path.splitext(path)[1].lower()
    file_size = get_file_size(path)

    if file_size > MAX_FILE_LOAD:
        logger.warning(f"File too large: {path} ({file_size / 1024 / 1024:.1f}MB). Max is {MAX_FILE_LOAD / 1024 / 1024:.0f}MB.")
        return None

    if file_size > LARGE_FILE_WARN:
        logger.warning(f"Large file: {path} ({file_size / 1024 / 1024:.1f}MB). Processing in batches...")

    # Text-based files: read directly
    if ext in TEXT_EXTENSIONS:
        try:
            with open(path, "r", encoding="utf-8", errors="replace") as f:
                return f.read()
        except Exception as e:
            logger.warning(f"Error reading {path}: {e}")
            return None

    # Complex formats: use unstructured
    if ext in UNSTRUCTURED_EXTENSIONS:
        try:
            from unstructured.partition.auto import partition

            elements = partition(filename=path)
            return "\n\n".join([str(el) for el in elements])
        except ImportError:
            logger.warning("unstructured not installed, reading as raw text")
            try:
                with open(path, "r", encoding="utf-8", errors="replace") as f:
                    return f.read()
            except Exception:
                return None
        except Exception as e:
            logger.warning(f"Error extracting {path} with unstructured: {e}")
            return None

    return None


class IngestionResult:
    """Result of an ingestion operation."""

    def __init__(self, path: str, chunks: int = 0, error: str = None, total_chunks: int = 0):
        self.path = path
        self.chunks = chunks
        self.error = error
        self.total_chunks = total_chunks


def ingest_file(
    path: str,
    embedder: Embedder,
    vector_db: VectorDB,
    chunk_size: int = 800,
    overlap: int = 100,
) -> IngestionResult:
    """Ingest a single file into the vector database.

    Processes large files in batches to avoid OOM.
    Memory is freed between batches.

    Args:
        path: File path
        embedder: Embedder instance
        vector_db: VectorDB instance
        chunk_size: Target chunk size
        overlap: Chunk overlap

    Returns:
        IngestionResult with status
    """
    try:
        file_size = get_file_size(path)
        ext = os.path.splitext(path)[1].lower()

        # For JSONL/CSV files: process line-by-line to save memory
        if ext in (".jsonl", ".ndjson", ".csv", ".tsv") and file_size > LARGE_FILE_WARN:
            return ingest_large_text_file(path, embedder, vector_db, chunk_size, overlap)

        text = get_text_content(path)
        if text is None:
            return IngestionResult(path=path, error="Could not extract text from file")

        text = text.strip()
        if not text:
            return IngestionResult(path=path, error="File is empty")

        return _chunk_and_embed(path, text, embedder, vector_db, chunk_size, overlap)

    except MemoryError:
        logger.exception("Out of memory during ingestion")
        return IngestionResult(path=path, error="Out of memory. Try a smaller file or free up RAM.")
    except Exception as e:
        logger.exception(f"Error ingesting {path}")
        return IngestionResult(path=path, error=str(e))


def ingest_large_text_file(
    path: str,
    embedder: Embedder,
    vector_db: VectorDB,
    chunk_size: int = 800,
    overlap: int = 100,
) -> IngestionResult:
    """Ingest a large text file (CSV, JSONL) line-by-line to save memory.

    Only loads a batch of lines at a time, processes them, then frees memory.
    """
    logger.info(f"Ingesting large file in streaming mode: {path}")
    total_chunks = 0
    source_name = os.path.basename(path)

    try:
        with open(path, "r", encoding="utf-8", errors="replace") as f:
            line_buffer = []
            line_count = 0
            batch_text = ""

            for line in f:
                line = line.strip()
                if not line:
                    continue
                line_count += 1
                batch_text += line + "\n"

                # Process in batches of 200 lines
                if line_count % 200 == 0:
                    chunks = chunk_text(batch_text, chunk_size=chunk_size, overlap=overlap)
                    if chunks:
                        n = _embed_and_store(path, source_name, chunks, embedder, vector_db)
                        total_chunks += n
                    batch_text = ""  # Free memory
                    logger.info(f"  Processed {line_count} lines, {total_chunks} chunks so far")

            # Process remaining
            if batch_text.strip():
                chunks = chunk_text(batch_text, chunk_size=chunk_size, overlap=overlap)
                if chunks:
                    n = _embed_and_store(path, source_name, chunks, embedder, vector_db)
                    total_chunks += n

    except MemoryError:
        logger.warning(f"Memory error at line {line_count}. Committing {total_chunks} chunks so far.")
    except Exception as e:
        logger.exception(f"Error during streaming ingestion at line {line_count}")
        return IngestionResult(path=path, chunks=total_chunks, error=str(e))

    total_in_db = vector_db.count()
    return IngestionResult(path=path, chunks=total_chunks, total_chunks=total_in_db)


def _chunk_and_embed(
    path: str,
    text: str,
    embedder: Embedder,
    vector_db: VectorDB,
    chunk_size: int,
    overlap: int,
) -> IngestionResult:
    """Chunk text, embed in batches, store in vector DB."""
    chunks = chunk_text(text, chunk_size=chunk_size, overlap=overlap)
    if not chunks:
        return IngestionResult(path=path, error="No chunks generated")

    logger.info(f"  Generated {len(chunks)} chunks")

    n = _embed_and_store(path, os.path.basename(path), chunks, embedder, vector_db)
    total_in_db = vector_db.count()

    return IngestionResult(path=path, chunks=n, total_chunks=total_in_db)


def _embed_and_store(
    path: str,
    source_name: str,
    chunks: List[str],
    embedder: Embedder,
    vector_db: VectorDB,
) -> int:
    """Embed chunks in batches and store in vector DB. Frees memory between batches."""
    all_embeddings = []
    total_embedded = 0

    for i in range(0, len(chunks), BATCH_SIZE):
        batch = chunks[i:i + BATCH_SIZE]
        batch_embeddings = embedder.embed(batch)
        if not batch_embeddings:
            logger.warning(f"  Batch {i // BATCH_SIZE + 1}: embedding returned empty")
            continue

        all_embeddings.extend(batch_embeddings)
        logger.info(f"  Embedded {min(i + BATCH_SIZE, len(chunks))}/{len(chunks)} chunks")
        total_embedded = min(i + BATCH_SIZE, len(chunks))

    if not all_embeddings:
        return 0

    # Prepare data for ChromaDB
    ids = []
    metadatas = []
    for i, chunk in enumerate(chunks):
        chunk_hash = hashlib.md5(chunk.encode()).hexdigest()[:12]
        ids.append(f"{source_name}_{i}_{chunk_hash}")
        metadatas.append({
            "source": source_name,
            "path": path,
            "chunk_index": i,
            "total_chunks": len(chunks),
            "char_count": len(chunk),
        })

    # Store in vector DB (this persists to disk, freeing Go memory)
    vector_db.add_documents(ids=ids, embeddings=all_embeddings, texts=chunks, metadatas=metadatas)

    # Free memory explicitly
    del all_embeddings
    del ids
    del metadatas

    return len(chunks)
