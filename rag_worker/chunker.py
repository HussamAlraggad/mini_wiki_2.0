"""
Chunker module: splits text into overlapping chunks for embedding.
Uses recursive character splitting with configurable chunk size and overlap.
"""

from typing import List


def chunk_text(text: str, chunk_size: int = 1500, overlap: int = 200) -> List[str]:
    """Split text into overlapping chunks using recursive character splitting.

    Args:
        text: Text to split
        chunk_size: Target chunk size in characters
        overlap: Overlap between consecutive chunks

    Returns:
        List of text chunks
    """
    if not text or not text.strip():
        return []

    # Normalize whitespace
    text = " ".join(text.split())

    # If text is shorter than chunk_size, return as single chunk
    if len(text) <= chunk_size:
        return [text]

    chunks = []
    separators = ["\n\n", "\n", ". ", "! ", "? ", ", ", " "]

    start = 0
    while start < len(text):
        end = min(start + chunk_size, len(text))

        # If we're not at the end, try to break at a separator
        if end < len(text):
            # Look for best separator within the last overlap characters
            best_pos = end
            for sep in separators:
                pos = text.rfind(sep, start + chunk_size - overlap, end)
                if pos > start + chunk_size - overlap:
                    best_pos = pos + len(sep)
                    break

            # If no separator found, just hard-break at chunk_size
            if best_pos == end:
                best_pos = end

            chunk = text[start:best_pos].strip()
            if chunk:
                chunks.append(chunk)

            start = best_pos
        else:
            chunk = text[start:end].strip()
            if chunk:
                chunks.append(chunk)
            break

    return chunks


def chunk_by_lines(text: str, max_lines: int = 20, overlap_lines: int = 3) -> List[str]:
    """Split text into chunks by lines (for structured text)."""
    lines = text.split("\n")
    if len(lines) <= max_lines:
        return [text]

    chunks = []
    i = 0
    while i < len(lines):
        end = min(i + max_lines, len(lines))
        chunk = "\n".join(lines[i:end]).strip()
        if chunk:
            chunks.append(chunk)
        i += max_lines - overlap_lines

    return chunks
