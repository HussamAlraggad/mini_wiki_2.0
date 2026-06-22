"""
OCR module: extracts text from scanned documents and images using Tesseract.
Used as a fallback when unstructured extraction returns empty text.

Requires tesseract-ocr installed on the system:
  sudo apt install tesseract-ocr         # Debian/Ubuntu
  brew install tesseract                 # macOS
  pacman -S tesseract                    # Arch
"""

import logging
import os
import subprocess
import tempfile

logger = logging.getLogger(__name__)

# Image formats that Tesseract can process directly
IMAGE_EXTENSIONS = {".png", ".jpg", ".jpeg", ".tiff", ".tif", ".bmp", ".pnm", ".webp"}


def is_tesseract_available() -> bool:
    """Check if Tesseract OCR is installed on the system."""
    try:
        result = subprocess.run(
            ["tesseract", "--version"],
            capture_output=True,
            text=True,
            timeout=5,
        )
        return result.returncode == 0
    except (FileNotFoundError, subprocess.TimeoutExpired):
        return False


def extract_text_with_ocr(filepath: str, lang: str = "eng") -> str:
    """Extract text from a scanned document or image using Tesseract OCR.

    Args:
        filepath: Path to the image or PDF file
        lang: Tesseract language code (default: eng)

    Returns:
        Extracted text, or empty string if extraction fails
    """
    if not is_tesseract_available():
        logger.warning(
            "Tesseract OCR is not installed. "
            "Install with: sudo apt install tesseract-ocr"
        )
        return ""

    if not os.path.exists(filepath):
        logger.warning(f"File not found: {filepath}")
        return ""

    ext = os.path.splitext(filepath)[1].lower()

    try:
        # For PDFs, convert to images first using pdftoppm if available,
        # otherwise pass directly to tesseract
        if ext == ".pdf":
            return _ocr_pdf(filepath, lang)
        elif ext in IMAGE_EXTENSIONS:
            return _ocr_image(filepath, lang)
        else:
            logger.warning(f"Unsupported format for OCR: {ext}")
            return ""
    except Exception as e:
        logger.warning(f"OCR extraction failed for {filepath}: {e}")
        return ""


def _ocr_image(filepath: str, lang: str) -> str:
    """Run Tesseract OCR on a single image file."""
    result = subprocess.run(
        ["tesseract", filepath, "stdout", "-l", lang, "--psm", "3"],
        capture_output=True,
        text=True,
        timeout=60,
    )
    if result.returncode != 0:
        logger.warning(f"Tesseract failed on {filepath}: {result.stderr}")
        return ""
    return result.stdout.strip()


def _ocr_pdf(filepath: str, lang: str) -> str:
    """OCR a PDF by converting pages to images first.

    Uses pdftoppm (poppler-utils) for PDF-to-image conversion,
    falls back to treating the PDF as a single image.
    """
    # Try pdftoppm first for multi-page PDFs
    try:
        with tempfile.TemporaryDirectory() as tmpdir:
            # Convert PDF pages to PNG images
            result = subprocess.run(
                ["pdftoppm", "-png", "-r", "300", filepath, f"{tmpdir}/page"],
                capture_output=True,
                text=True,
                timeout=120,
            )
            if result.returncode == 0:
                # Collect all page images
                pages = sorted([
                    os.path.join(tmpdir, f)
                    for f in os.listdir(tmpdir)
                    if f.endswith(".png")
                ])
                if pages:
                    texts = []
                    for page_path in pages:
                        page_text = _ocr_image(page_path, lang)
                        if page_text:
                            texts.append(page_text)
                    return "\n\n".join(texts)
    except FileNotFoundError:
        logger.warning("pdftoppm not found. Install poppler-utils for better PDF OCR.")
    except Exception as e:
        logger.warning(f"PDF OCR via pdftoppm failed: {e}")

    # Fallback: treat PDF as single image
    logger.warning("Falling back to single-image PDF OCR (first page only)")
    return _ocr_image(filepath, lang)


def is_scanned_document(text: str) -> bool:
    """Heuristic: check if extracted text looks like OCR output.

    Returns True if the text is very short relative to file size,
    or contains typical OCR artifacts like missing spaces.
    """
    if not text:
        return True  # Empty text from a non-trivial file suggests scanned
    # Check for very dense text without spaces (OCR often misses spaces)
    words = text.split()
    if len(words) == 0:
        return True
    avg_word_len = sum(len(w) for w in words) / len(words)
    # If average word length > 15, it's likely garbage/OCR without proper spaces
    return avg_word_len > 15
