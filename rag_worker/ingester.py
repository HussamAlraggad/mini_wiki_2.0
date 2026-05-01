"""
Ingester module: ingests documents into the vector database.
Supports PDF, TXT, MD, CSV, JSONL, and other formats via unstructured.
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


def get_text_content(path: str) -> Optional[str]:
    """Extract text content from a file using unstructured for complex formats.

    Args:
        path: Path to the file

    Returns:
        Extracted text, or None if extraction fails
    """
    ext = os.path.splitext(path)[1].lower()

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
            logger.warning("unstructured not installed, falling back to raw text")
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

    def __init__(self, path: str, chunks: int = 0, error: str = None):
        self.path = path
        self.chunks = chunks
        self.error = error


def ingest_file(
    path: str,
    embedder: Embedder,
    vector_db: VectorDB,
    chunk_size: int = 800,
    overlap: int = 100,
) -> IngestionResult:
    """Ingest a single file into the vector database.

    Steps:
    1. Extract text from file
    2. Split into chunks
    3. Generate embeddings
    4. Store in ChromaDB

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
        text = get_text_content(path)
        if text is None:
            return IngestionResult(path=path, error="Could not extract text from file")

        text = text.strip()
        if not text:
            return IngestionResult(path=path, error="File is empty")

        # Chunk
        chunks = chunk_text(text, chunk_size=chunk_size, overlap=overlap)
        if not chunks:
            return IngestionResult(path=path, error="No chunks generated")

        # Generate embeddings in batches
        batch_size = 50
        all_embeddings = []
        for i in range(0, len(chunks), batch_size):
            batch = chunks[i:i + batch_size]
            batch_embeddings = embedder.embed(batch)
            all_embeddings.extend(batch_embeddings)
            logger.info(f"  Embedded {min(i + batch_size, len(chunks))}/{len(chunks)} chunks")

        if not all_embeddings or len(all_embeddings) != len(chunks):
            return IngestionResult(path=path, error="Embedding failed")

        # Prepare data for ChromaDB
        source_name = os.path.basename(path)
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

        # Store in vector DB
        vector_db.add_documents(
            ids=ids,
            embeddings=all_embeddings,
            texts=chunks,
            metadatas=metadatas,
        )

        return IngestionResult(path=path, chunks=len(chunks))

    except Exception as e:
        logger.exception(f"Error ingesting {path}")
        return IngestionResult(path=path, error=str(e))
