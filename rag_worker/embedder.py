"""
Embedder module: generates embeddings via Ollama /api/embed endpoint.
Supports multiple embedding models for flexible quality/speed tradeoffs.
"""

import json
import logging
import time
from typing import List, Optional
from urllib.request import Request, urlopen
from urllib.error import URLError

logger = logging.getLogger(__name__)


class Embedder:
    """Generates embeddings using Ollama's /api/embed endpoint."""

    def __init__(self, model: str = "nomic-embed-text", base_url: str = "http://127.0.0.1:11434"):
        self.model = model
        self.base_url = base_url.rstrip("/")

    def embed(self, texts: List[str]) -> List[List[float]]:
        """Generate embeddings for a list of texts.

        Args:
            texts: List of text strings to embed

        Returns:
            List of embedding vectors (each a list of floats)

        Raises:
            RuntimeError: If Ollama is not reachable or returns an error
        """
        if not texts:
            return []

        # Filter out empty strings
        valid_texts = [t for t in texts if t and t.strip()]
        if not valid_texts:
            return [[] for _ in texts]

        url = f"{self.base_url}/api/embed"
        payload = json.dumps({"model": self.model, "input": valid_texts}).encode("utf-8")

        for attempt in range(3):
            try:
                req = Request(url, data=payload, headers={"Content-Type": "application/json"})
                resp = urlopen(req, timeout=60)
                result = json.loads(resp.read().decode("utf-8"))

                if "embeddings" not in result:
                    raise RuntimeError(f"Unexpected response: {result}")

                return result["embeddings"]

            except URLError as e:
                if attempt < 2:
                    wait = 1.0 * (attempt + 1)
                    logger.warning(f"Embedding attempt {attempt + 1} failed: {e}. Retrying in {wait}s...")
                    time.sleep(wait)
                else:
                    raise RuntimeError(f"Cannot reach Ollama at {self.base_url}: {e}")
            except json.JSONDecodeError as e:
                raise RuntimeError(f"Invalid JSON from Ollama: {e}")

        return []

    def embed_single(self, text: str) -> List[float]:
        """Embed a single text string."""
        results = self.embed([text])
        return results[0] if results else []

    @property
    def dimension(self) -> int:
        """Get the embedding dimension for the current model."""
        sample = self.embed_single("test")
        return len(sample) if sample else 0
