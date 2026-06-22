"""
Ingester module: ingests documents into the vector database.
Handles large files safely by streaming content and processing in batches.

Supports:
- Plain text files (.txt, .md, .json, .csv, .tsv, .jsonl, etc.)
- Office documents (.pdf, .docx, .pptx, .xlsx) via unstructured or LangChain
- Web content (.html, .htm, .xml, .epub)
- Scanned documents via Tesseract OCR fallback
- Extended formats via LangChain document loaders
"""

import hashlib
import logging
import os
import time
from typing import List, Optional

from rag_worker.chunker import chunk_text
from rag_worker.embedder import Embedder
from rag_worker.vectordb import VectorDB
from rag_worker.ocr import extract_text_with_ocr, is_scanned_document, is_tesseract_available

logger = logging.getLogger(__name__)

# File extensions that unstructured handles
UNSTRUCTURED_EXTENSIONS = {".pdf", ".docx", ".doc", ".html", ".htm", ".xml", ".epub", ".odt", ".ppt", ".pptx", ".xls", ".xlsx"}

# Text-based files we handle directly
TEXT_EXTENSIONS = {".txt", ".md", ".markdown", ".rst", ".json", ".yaml", ".yml", ".toml", ".cfg", ".ini", ".conf", ".env", ".csv", ".tsv", ".jsonl", ".ndjson", ".log", ".sql", ".sh", ".py", ".go", ".js", ".ts", ".xml"}

# Image/scan formats handled by Tesseract OCR
OCR_EXTENSIONS = {".png", ".jpg", ".jpeg", ".tiff", ".tif", ".bmp", ".pnm", ".webp"}

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
    """Extract text content from a file using the best available method.

    Pipeline:
    1. Direct read for plain text files
    2. LangChain document loaders (if available) for complex formats
    3. Unstructured library (fallback for PDFs, DOCX, etc.)
    4. Tesseract OCR (final fallback for scanned documents)

    Args:
        path: Path to the file

    Returns:
        Extracted text, or None if all methods fail
    """
    ext = os.path.splitext(path)[1].lower()
    file_size = get_file_size(path)

    if file_size > MAX_FILE_LOAD:
        logger.warning(f"File too large: {path} ({file_size / 1024 / 1024:.1f}MB). Max is {MAX_FILE_LOAD / 1024 / 1024:.0f}MB.")
        return None

    if file_size > LARGE_FILE_WARN:
        logger.warning(f"Large file: {path} ({file_size / 1024 / 1024:.1f}MB). Processing in batches...")

    # 1. Text-based files: read directly
    if ext in TEXT_EXTENSIONS:
        try:
            with open(path, "r", encoding="utf-8", errors="replace") as f:
                return f.read()
        except Exception as e:
            logger.warning(f"Error reading {path}: {e}")
            return None

    # 2. Complex formats: try LangChain document loaders first
    if ext in UNSTRUCTURED_EXTENSIONS:
        text = _try_langchain_load(path, ext)
        if text:
            return text

        # 3. Fallback to unstructured
        text = _try_unstructured_load(path)
        if text:
            return text

        # 4. Final fallback: OCR for scanned documents/images
        if ext in (".pdf",) or ext in OCR_EXTENSIONS:
            if is_tesseract_available():
                logger.info(f"Attempting OCR on {path}...")
                ocr_text = extract_text_with_ocr(path)
                if ocr_text and not is_scanned_document(ocr_text):
                    return ocr_text
                elif ocr_text:
                    logger.info(f"OCR extracted text from {path} ({len(ocr_text)} chars)")

    # 5. Image files: try OCR directly
    if ext in OCR_EXTENSIONS:
        if is_tesseract_available():
            logger.info(f"Running OCR on image: {path}")
            return extract_text_with_ocr(path) or None

    return None


def _try_langchain_load(path: str, ext: str) -> Optional[str]:
    """Try to load a document using LangChain document loaders."""
    try:
        from langchain_community.document_loaders import (
            PDFLoader,
            Docx2txtLoader,
            UnstructuredHTMLLoader,
            UnstructuredXMLLoader,
            UnstructuredEPubLoader,
            UnstructuredPowerPointLoader,
            UnstructuredExcelLoader,
        )

        loader_map = {
            ".pdf": lambda p: PDFLoader(p),
            ".docx": lambda p: Docx2txtLoader(p),
            ".doc": lambda p: Docx2txtLoader(p),
            ".html": lambda p: UnstructuredHTMLLoader(p),
            ".htm": lambda p: UnstructuredHTMLLoader(p),
            ".xml": lambda p: UnstructuredXMLLoader(p),
            ".epub": lambda p: UnstructuredEPubLoader(p),
            ".ppt": lambda p: UnstructuredPowerPointLoader(p),
            ".pptx": lambda p: UnstructuredPowerPointLoader(p),
            ".xls": lambda p: UnstructuredExcelLoader(p),
            ".xlsx": lambda p: UnstructuredExcelLoader(p),
        }

        loader_fn = loader_map.get(ext)
        if loader_fn:
            loader = loader_fn(path)
            documents = loader.load()
            if documents:
                text = "\n\n".join([doc.page_content for doc in documents])
                logger.info(f"LangChain loaded {path}: {len(documents)} pages, {len(text)} chars")
                return text
    except ImportError:
        logger.debug("LangChain loaders not available, falling back to unstructured")
    except Exception as e:
        logger.warning(f"LangChain loader failed for {path}: {e}")
    return None


def _try_unstructured_load(path: str) -> Optional[str]:
    """Try to extract text using the unstructured library."""
    try:
        from unstructured.partition.auto import partition

        elements = partition(filename=path)
        text = "\n\n".join([str(el) for el in elements])
        if text.strip():
            return text
    except ImportError:
        logger.warning("unstructured not available. Install with: pip install unstructured[pdf,docx,md]")
    except Exception as e:
        logger.warning(f"Unstructured extraction failed for {path}: {e}")

    # Last resort: read as raw text
    try:
        with open(path, "r", encoding="utf-8", errors="replace") as f:
            return f.read()
    except Exception:
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
    chunk_size: int = 1500,
    overlap: int = 200,
    progress_callback=None,
    deep_read_func=None,
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
            result = ingest_large_text_file(path, embedder, vector_db, chunk_size, overlap, progress_callback=progress_callback, deep_read_func=deep_read_func)
            result.progress = progress + result.progress
            return result

        text = get_text_content(path)
        if text is None:
            return IngestionResult(path=path, error="Could not extract text from file", progress=progress)

        text = text.strip()
        if not text:
            return IngestionResult(path=path, error="File is empty", progress=progress)

        result = _chunk_and_embed(path, text, embedder, vector_db, chunk_size, overlap, deep_read_func=deep_read_func)
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
    chunk_size: int = 1500,
    overlap: int = 200,
    progress_callback=None,
    deep_read_func=None,
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
                n = _embed_and_store(path, source_name, chunks, embedder, vector_db, progress_callback, deep_read_func)
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
    deep_read_func=None,
) -> IngestionResult:
    """Chunk text, embed in batches, store in vector DB."""
    progress = []
    progress.append(f"Chunking {len(text)} characters...")
    chunks = chunk_text(text, chunk_size=chunk_size, overlap=overlap)
    if not chunks:
        return IngestionResult(path=path, error="No chunks generated", progress=progress)

    progress.append(f"Generated {len(chunks)} chunks")
    progress.append(f"Embedding {len(chunks)} chunks...")

    n = _embed_and_store(path, os.path.basename(path), chunks, embedder, vector_db, deep_read_func=deep_read_func)
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
    deep_read_func=None,
) -> int:
    """Embed chunks in batches and store in vector DB. Frees memory between batches.

    If deep_read_func is provided, each chunk is also sent through a 'deep reading'
    LLM that produces a human-like understanding. Both the original chunk and the
    AI's understanding are embedded and stored.
    """
    all_embeddings = []
    all_texts = []
    all_ids = []
    all_metadatas = []
    total_embedded = 0
    source_base = source_name

    for i in range(0, len(chunks), BATCH_SIZE):
        batch = chunks[i:i + BATCH_SIZE]
        if i % (BATCH_SIZE * 10) == 0 and progress_callback:
            done = min(i + BATCH_SIZE, len(chunks))
            progress_callback(f"  Embedding {done}/{len(chunks)} chunks...")

        # If deep reading is enabled, process each chunk through the LLM first
        if deep_read_func:
            understanding_texts = []
            for ci, chunk in enumerate(batch):
                abs_idx = i + ci
                if progress_callback and ci % 5 == 0:
                    progress_callback(f"  Deep reading chunk {abs_idx + 1}/{len(chunks)}...")
                try:
                    understanding = deep_read_func(chunk)
                    understanding_texts.append(understanding)
                except Exception as e:
                    logger.warning(f"  Deep read failed for chunk {abs_idx + 1}: {e}")
                    understanding_texts.append(chunk)  # fallback: use original

            # Embed understanding texts alongside originals
            all_understandings = embedder.embed(understanding_texts)
            if all_understandings:
                for ci, (chunk, understanding, emb) in enumerate(zip(batch, understanding_texts, all_understandings)):
                    abs_idx = i + ci
                    chunk_hash = hashlib.md5(chunk.encode()).hexdigest()[:12]
                    # Store original chunk
                    all_ids.append(f"{source_base}_{abs_idx}_{chunk_hash}")
                    all_texts.append(chunk)
                    all_embeddings.append(emb)
                    all_metadatas.append({
                        "source": source_base,
                        "path": path,
                        "chunk_index": abs_idx,
                        "type": "original",
                        "char_count": len(chunk),
                    })
                    # Store understanding as a separate entry
                    understanding_hash = hashlib.md5(understanding.encode()).hexdigest()[:12]
                    u_emb = embedder.embed([understanding])[0]
                    all_ids.append(f"{source_base}_{abs_idx}_{understanding_hash}_deep")
                    all_texts.append(understanding)
                    all_embeddings.append(u_emb)
                    all_metadatas.append({
                        "source": source_base,
                        "path": path,
                        "chunk_index": abs_idx,
                        "type": "deep_understanding",
                        "char_count": len(understanding),
                    })
                total_embedded = min(i + BATCH_SIZE, len(chunks)) * 2
                continue

        # Standard path: batch embed as usual
        batch_embeddings = embedder.embed(batch)
        if not batch_embeddings:
            logger.warning(f"  Batch {i // BATCH_SIZE + 1}: embedding returned empty")
            continue

        for ci, chunk in enumerate(batch):
            abs_idx = i + ci
            chunk_hash = hashlib.md5(chunk.encode()).hexdigest()[:12]
            all_ids.append(f"{source_base}_{abs_idx}_{chunk_hash}")
            all_texts.append(chunk)
            all_embeddings.append(batch_embeddings[ci] if ci < len(batch_embeddings) else [0.0])
            all_metadatas.append({
                "source": source_base,
                "path": path,
                "chunk_index": abs_idx,
                "type": "original",
                "char_count": len(chunk),
            })
        done = min(i + BATCH_SIZE, len(chunks))
        logger.info(f"  Embedded {done}/{len(chunks)} chunks")
        total_embedded = done

    if not all_embeddings:
        return 0

    # Store all accumulated data (originals + understandings if deep)
    if all_ids:
        vector_db.add_documents(
            ids=all_ids,
            embeddings=all_embeddings,
            texts=all_texts,
            metadatas=all_metadatas,
        )

    # Free memory explicitly
    del all_embeddings
    del all_ids
    del all_texts
    del all_metadatas

    return len(all_ids)
