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

    def __init__(self, path: str, chunks: int = 0, error: str = None, total_chunks: int = 0, progress: list = None):
        self.path = path
        self.chunks = chunks
        self.error = error
        self.total_chunks = total_chunks
        self.progress = progress or []


def ingest_file(
    path: str,
    embedder: Embedder,
    vector_db: VectorDB,
    chunk_size: int = 4000,
    overlap: int = 400,
    progress_callback=None,
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
    progress = []

    try:
        file_size = get_file_size(path)
        ext = os.path.splitext(path)[1].lower()

        progress.append(f"Reading {os.path.basename(path)} ({file_size / 1024:.0f} KB)...")
        # For JSONL/CSV files: process line-by-line to save memory
        if ext in (".jsonl", ".ndjson", ".csv", ".tsv") and file_size > LARGE_FILE_WARN:
            result = ingest_large_text_file(path, embedder, vector_db, chunk_size, overlap, progress_callback=progress_callback)
            result.progress = progress + result.progress
            return result

        text = get_text_content(path)
        if text is None:
            return IngestionResult(path=path, error="Could not extract text from file", progress=progress)

        text = text.strip()
        if not text:
            return IngestionResult(path=path, error="File is empty", progress=progress)

        result = _chunk_and_embed(path, text, embedder, vector_db, chunk_size, overlap)
        result.progress = progress + result.progress
        return result

    except MemoryError:
        logger.exception("Out of memory during ingestion")
        return IngestionResult(path=path, error="Out of memory. Try a smaller file or free up RAM.", progress=progress)
    except Exception as e:
        logger.exception(f"Error ingesting {path}")
        return IngestionResult(path=path, error=str(e), progress=progress)


def count_lines_in_file(path: str) -> int:
    """Quickly count non-empty lines in a file (same as countFileRows in Go)."""
    count = 0
    try:
        with open(path, "r", encoding="utf-8", errors="replace") as f:
            for line in f:
                if line.strip():
                    count += 1
    except Exception:
        pass
    return count


def ingest_large_text_file(
    path: str,
    embedder: Embedder,
    vector_db: VectorDB,
    chunk_size: int = 4000,
    overlap: int = 400,
    progress_callback=None,
) -> IngestionResult:
    """Ingest a large text file (CSV, JSONL) one line at a time.

    Each line is chunked and embedded independently. This ensures chunks
    never cross record boundaries and gives accurate progress tracking.
    Shows estimated time remaining based on per-line timing.
    """
    logger.info(f"Ingesting large file one line at a time: {path}")
    source_name = os.path.basename(path)
    progress = []

    # Count total lines for progress (fast: 1.1GB in <1s)
    total_lines = count_lines_in_file(path)
    if total_lines == 0:
        return IngestionResult(path=path, error="File is empty", progress=progress)

    if progress_callback:
        progress_callback(f"File has {total_lines} lines. Embedding each line independently...")

    total_chunks = 0
    start_time = time.time()
    line_count = 0
    embedded_chunks_total = 0

    try:
        with open(path, "r", encoding="utf-8", errors="replace") as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue
                line_count += 1

                # Show progress before each line (updates the idle timer)
                if progress_callback:
                    elapsed = time.time() - start_time
                    if elapsed > 0 and line_count > 1:
                        rate = (line_count - 1) / elapsed
                        remaining = (total_lines - line_count + 1) / rate
                        eta = ""
                        if remaining > 60:
                            eta = f" (est. {remaining/60:.0f}m remaining)"
                        else:
                            eta = f" (est. {remaining:.0f}s remaining)"
                        progress_callback(f"Line {line_count}/{total_lines}, {total_chunks} chunks{eta}")
                    else:
                        progress_callback(f"Line {line_count}/{total_lines}...")

                # Chunk this single line
                chunks = chunk_text(line, chunk_size=chunk_size, overlap=overlap)
                if not chunks:
                    continue

                # Embed and store this line's chunks
                n = _embed_and_store(path, source_name, chunks, embedder, vector_db, progress_callback)
                total_chunks += n
                embedded_chunks_total += len(chunks)

        msg = f"  Finished: {line_count} lines, {total_chunks} chunks indexed"
        logger.info(msg)
        if progress_callback:
            progress_callback(msg)

    except MemoryError:
        logger.warning(f"Memory error at line {line_count}. Committing {total_chunks} chunks so far.")
    except Exception as e:
        logger.exception(f"Error during streaming ingestion at line {line_count}")
        return IngestionResult(path=path, chunks=total_chunks, error=str(e), progress=progress)

    total_in_db = vector_db.count()
    return IngestionResult(path=path, chunks=total_chunks, total_chunks=total_in_db, progress=progress)


def _chunk_and_embed(
    path: str,
    text: str,
    embedder: Embedder,
    vector_db: VectorDB,
    chunk_size: int,
    overlap: int,
) -> IngestionResult:
    """Chunk text, embed in batches, store in vector DB."""
    progress = []
    progress.append(f"Chunking {len(text)} characters...")
    chunks = chunk_text(text, chunk_size=chunk_size, overlap=overlap)
    if not chunks:
        return IngestionResult(path=path, error="No chunks generated", progress=progress)

    progress.append(f"Generated {len(chunks)} chunks")
    progress.append(f"Embedding {len(chunks)} chunks...")

    n = _embed_and_store(path, os.path.basename(path), chunks, embedder, vector_db)
    total_in_db = vector_db.count()

    progress.append(f"Stored {n} chunks in database")
    return IngestionResult(path=path, chunks=n, total_chunks=total_in_db, progress=progress)


def _embed_and_store(
    path: str,
    source_name: str,
    chunks: List[str],
    embedder: Embedder,
    vector_db: VectorDB,
    progress_callback=None,
) -> int:
    """Embed chunks in batches and store in vector DB. Frees memory between batches."""
    all_embeddings = []
    total_embedded = 0

    for i in range(0, len(chunks), BATCH_SIZE):
        batch = chunks[i:i + BATCH_SIZE]
        # Send progress every 10 batches to keep the Go idle timer alive
        if i % (BATCH_SIZE * 10) == 0 and progress_callback:
            done = min(i + BATCH_SIZE, len(chunks))
            progress_callback(f"  Embedding {done}/{len(chunks)} chunks...")
        batch_embeddings = embedder.embed(batch)
        if not batch_embeddings:
            logger.warning(f"  Batch {i // BATCH_SIZE + 1}: embedding returned empty")
            continue

        all_embeddings.extend(batch_embeddings)
        done = min(i + BATCH_SIZE, len(chunks))
        logger.info(f"  Embedded {done}/{len(chunks)} chunks")
        total_embedded = done

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
