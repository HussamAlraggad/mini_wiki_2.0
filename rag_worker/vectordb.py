"""
Vector DB module: wraps ChromaDB for persistent vector storage on disk.
Stores embeddings, text chunks, and metadata (source file, page number, etc.).
"""

import json
import logging
import os
from typing import List, Optional, Dict, Any

import chromadb
from chromadb.config import Settings

logger = logging.getLogger(__name__)


class VectorDB:
    """Persistent vector database using ChromaDB."""

    def __init__(self, path: str):
        """Initialize ChromaDB at the given path.

        Args:
            path: Directory path for persistent storage (e.g., .wiki/rag/)
        """
        os.makedirs(path, exist_ok=True)
        self.path = path
        self.client = chromadb.PersistentClient(
            path=path,
            settings=Settings(anonymized_telemetry=False),
        )
        self._collection = None

    @property
    def collection(self):
        """Get or create the default collection."""
        if self._collection is None:
            try:
                self._collection = self.client.get_collection("documents")
            except Exception:
                # Collection may not exist (newer ChromaDB raises NotFoundError)
                self._collection = self.client.create_collection("documents")
        return self._collection

    def add_documents(
        self,
        ids: List[str],
        embeddings: List[List[float]],
        texts: List[str],
        metadatas: List[Dict[str, Any]],
    ) -> int:
        """Add documents to the vector store.

        Args:
            ids: Unique IDs for each chunk
            embeddings: Embedding vectors
            texts: Original text chunks
            metadatas: Metadata dicts (source, chunk_index, etc.)

        Returns:
            Number of documents added
        """
        if not ids:
            return 0

        # Flatten metadata values to strings for ChromaDB compatibility
        clean_metadatas = []
        for m in metadatas:
            clean = {}
            for k, v in m.items():
                if isinstance(v, (str, int, float, bool)):
                    clean[k] = v
                else:
                    clean[k] = json.dumps(v)
            clean_metadatas.append(clean)

        self.collection.add(
            ids=ids,
            embeddings=embeddings,
            documents=texts,
            metadatas=clean_metadatas,
        )
        return len(ids)

    def search(
        self, query_embedding: List[float], top_k: int = 5
    ) -> List[Dict[str, Any]]:
        """Search for similar documents by embedding.

        Args:
            query_embedding: Query embedding vector
            top_k: Number of results to return

        Returns:
            List of result dicts with document, metadata, and distance
        """
        results = self.collection.query(
            query_embeddings=[query_embedding],
            n_results=min(top_k, self.count()),
        )

        documents = results["documents"][0] if results["documents"] else []
        metadatas = results["metadatas"][0] if results["metadatas"] else []
        distances = results["distances"][0] if results["distances"] else []

        output = []
        for doc, meta, dist in zip(documents, metadatas, distances):
            output.append({
                "text": doc,
                "metadata": meta,
                "score": 1.0 - dist / 2.0,  # Normalize to [0, 1]
                "distance": float(dist),
            })

        return output

    def count(self) -> int:
        """Get total number of indexed chunks."""
        return self.collection.count()

    def list_sources(self) -> List[str]:
        """List all unique source files in the index."""
        results = self.collection.get()
        sources = set()
        for meta in results["metadatas"]:
            if "source" in meta:
                sources.add(meta["source"])
        return sorted(sources)

    def delete_source(self, source: str) -> int:
        """Delete all chunks from a specific source file.

        Args:
            source: Source filename to delete

        Returns:
            Number of chunks deleted
        """
        results = self.collection.get()
        ids_to_delete = []
        for i, meta in enumerate(results["metadatas"]):
            if meta.get("source") == source:
                ids_to_delete.append(results["ids"][i])

        if ids_to_delete:
            self.collection.delete(ids=ids_to_delete)

        return len(ids_to_delete)

    def delete_all(self) -> int:
        """Delete all documents from the collection.

        Returns:
            Number of documents deleted
        """
        count = self.count()
        if count > 0:
            self.client.delete_collection("documents")
            self._collection = None
        return count
