"""
Reranker module: re-scores retrieved chunks using a cross-encoder model.
Improves precision by directly comparing query vs each candidate chunk
with full attention, catching nuances that vector similarity misses.

Uses the lightweight MiniLM model (22M params, ~80MB) which runs on
CPU in ~50ms for 10 chunks. No GPU needed.
"""

import logging
from typing import Any, Dict, List, Optional

logger = logging.getLogger(__name__)


class Reranker:
    """Cross-encoder reranker for improving RAG retrieval precision.

    Usage:
        reranker = Reranker()
        chunks = reranker.rerank(query, raw_chunks, top_k=5)

    The model is loaded lazily on first use to avoid import overhead
    when reranking is not needed.
    """

    def __init__(self, model_name: str = "cross-encoder/ms-marco-MiniLM-L-6-v2"):
        self.model_name = model_name
        self._model = None

    def _load_model(self):
        """Lazy-load the cross-encoder model."""
        if self._model is not None:
            return
        try:
            from sentence_transformers import CrossEncoder
            self._model = CrossEncoder(self.model_name)
            logger.info(f"Reranker loaded: {self.model_name}")
        except ImportError:
            logger.warning(
                "sentence-transformers not installed. "
                "Install with: pip install sentence-transformers"
            )
            self._model = None
        except Exception as e:
            logger.warning(f"Failed to load reranker model: {e}")
            self._model = None

    @property
    def available(self) -> bool:
        """Check if the reranker model is loaded and usable."""
        self._load_model()
        return self._model is not None

    def rerank(
        self,
        query: str,
        chunks: List[Dict[str, Any]],
        top_k: int = 5,
    ) -> List[Dict[str, Any]]:
        """Re-score chunks by query-chunk relevance using a cross-encoder.

        Args:
            query: The user's question
            chunks: List of chunk dicts with 'text' key
            top_k: Number of top chunks to return after reranking

        Returns:
            Re-ranked chunks with updated 'score' field and 'rerank_score' field
        """
        self._load_model()
        if self._model is None or not chunks:
            return chunks[:top_k]

        # Build query-chunk pairs
        pairs = []
        valid_chunks = []
        for chunk in chunks:
            text = chunk.get("text", "")
            if text and len(text.strip()) > 5:
                pairs.append((query, text))
                valid_chunks.append(chunk)

        if not pairs:
            return chunks[:top_k]

        try:
            # Score all pairs at once (batched)
            scores = self._model.predict(pairs, show_progress_bar=False)

            # Update chunks with reranker scores
            for i, chunk in enumerate(valid_chunks):
                score = float(scores[i]) if i < len(scores) else 0.0
                # Normalize from [0,1] range (sigmoid output) for consistency
                chunk["rerank_score"] = round(score, 4)
                # Blend original vector score with reranker score
                orig_score = chunk.get("score", 0.0)
                chunk["score"] = round((orig_score + score) / 2, 4)

            # Sort by blended score descending
            valid_chunks.sort(key=lambda c: c.get("score", 0.0), reverse=True)

            logger.debug(
                f"Reranked {len(valid_chunks)} chunks, "
                f"top score: {valid_chunks[0]['score'] if valid_chunks else 'N/A'}"
            )
            return valid_chunks[:top_k]

        except Exception as e:
            logger.warning(f"Reranking failed: {e}. Falling back to original scores.")
            return chunks[:top_k]
