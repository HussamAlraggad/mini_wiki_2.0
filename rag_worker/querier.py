"""
Querier module: embeds a query, searches the vector DB, retrieves context,
and sends to LLM for answer with sources.
"""

import json
import logging
import time
from typing import Any, Dict, List, Optional
from urllib.request import Request, urlopen
from urllib.error import URLError

from rag_worker.embedder import Embedder
from rag_worker.vectordb import VectorDB
from rag_worker.deep_reader import deep_read

logger = logging.getLogger(__name__)


class QueryResult:
    """Result of a RAG query."""

    def __init__(self, answer: str = "", sources: List[Dict] = None, model: str = "", tokens_used: int = 0):
        self.answer = answer
        self.sources = sources or []
        self.model = model
        self.tokens_used = tokens_used


def query(
    question: str,
    embedder: Embedder,
    vector_db: VectorDB,
    llm_model: str = "llama3.1:8b",
    top_k: int = 5,
    system_prompt: str = None,
    ollama_base: str = "http://127.0.0.1:11434",
    deep: bool = False,
    deep_model: str = "gemma4:e4b",
) -> QueryResult:
    """Run a RAG query: embed -> search -> retrieve -> LLM -> answer.

    When deep=True, retrieved chunks are first read by gemma4 like a human
    researcher, producing detailed understanding text that replaces raw chunks
    in the LLM context. Adds ~10-25 seconds per question but gives much richer,
    more human-like answers.

    Args:
        question: User's question
        embedder: Embedder instance
        vector_db: VectorDB instance
        llm_model: LLM model for answering
        top_k: Number of chunks to retrieve
        system_prompt: Override default system prompt
        ollama_base: Ollama base URL
        deep: If True, use on-demand deep reading of retrieved chunks
        deep_model: LLM model to use for deep reading

    Returns:
        QueryResult with answer and sources
    """
    # 1. Embed the query
    query_embedding = embedder.embed_single(question)
    if not query_embedding:
        return QueryResult(answer="Failed to embed query. Is Ollama running?", model=llm_model)

    # 2. Search vector DB
    results = vector_db.search(query_embedding, top_k=top_k)
    if not results:
        return QueryResult(
            answer="No relevant documents found in the knowledge base. Try ingesting files first with /ingest.",
            model=llm_model,
        )

    # 3. Retrieve chunks and optionally do deep reading
    sources = []
    context_parts = []

    # Limit deep reading to top 3 chunks to keep latency reasonable
    deep_limit = min(3, len(results)) if deep else 0

    for i, r in enumerate(results):
        source = r.get("metadata", {}).get("source", "unknown")
        score = r.get("score", 0)
        text = r.get("text", "")

        display_text = text[:300] + "..." if len(text) > 300 else text
        sources.append({
            "file": source,
            "score": round(score, 3),
            "text": display_text,
        })

        # Deep reading: gemma4 reads the chunk like a human
        if i < deep_limit:
            logger.warning(f"Deep reading chunk {i+1}/{deep_limit}...")
            try:
                understanding = deep_read(text, deep_model, ollama_base)
                context_parts.append(
                    f"SOURCE {i+1}: {source}\n"
                    f"[Deep analysis of this source by a research expert:]\n{understanding}"
                )
                continue
            except Exception as e:
                logger.warning(f"Deep read failed for chunk {i+1}: {e}")

        # Fallback: use cleaned chunk text
        clean_text = format_chunk_text(text, source)
        context_parts.append(f"SOURCE {i+1}: {source}\n{clean_text}")

    context = "\n\n---\n\n".join(context_parts)

    # 4. Build system prompt
    if system_prompt is None:
        system_prompt = (
            "You are a research assistant analyzing documents. "
            "Use the following context to answer the question. "
            "If the context does not contain enough information, say so clearly. "
            "Do not make up information. "
            "Cite the source file names when using specific information from the context."
        )

    if deep:
        system_prompt = (
            "You are a brilliant research assistant with access to both raw data "
            "and expert-level analyses of that data. The context below includes "
            "deep analyses written by a research expert who read the original sources. "
            "Use these analyses to provide a thorough, insightful, and human-like answer. "
            "Synthesize patterns, explain implications, and connect ideas. "
            "Cite sources when using specific information."
        )

    # 5. Call LLM with context
    answer = call_llm(
        model=llm_model,
        system=system_prompt,
        user_message=f"Context:\n{context}\n\nQuestion: {question}",
        base_url=ollama_base,
    )

    return QueryResult(answer=answer, sources=sources, model=llm_model)


def format_chunk_text(text: str, source: str = "") -> str:
    """Convert a raw text chunk into clean, readable text for the LLM.

    Handles:
    - JSON objects/arrays → formatted key-value pairs
    - CSV/TSV lines → formatted as readable records
    - Plain text → cleaned up (truncated if very long)
    """
    if not text:
        return "(empty)"

    text = text.strip()
    if not text:
        return "(empty)"

    # Limit chunk length for LLM context
    max_chunk_len = 2000
    if len(text) > max_chunk_len:
        text = text[:max_chunk_len] + "..."

    # Try to parse as JSON and format nicely
    try:
        parsed = json.loads(text)
        if isinstance(parsed, dict):
            lines = []
            for k, v in parsed.items():
                if isinstance(v, (list, dict)):
                    v_str = json.dumps(v, ensure_ascii=False)
                    if len(v_str) > 200:
                        v_str = v_str[:200] + "..."
                    lines.append(f"  {k}: {v_str}")
                else:
                    lines.append(f"  {k}: {v}")
            return "\n".join(lines)
        elif isinstance(parsed, list):
            items = []
            for item in parsed[:10]:
                if isinstance(item, dict):
                    items.append(format_chunk_text(json.dumps(item), source))
                else:
                    items.append(str(item))
            return "\n".join(items)
    except (json.JSONDecodeError, ValueError):
        pass

    return text


def call_llm(
    model: str,
    system: str,
    user_message: str,
    base_url: str = "http://127.0.0.1:11434",
    temperature: float = 0.1,
) -> str:
    """Call Ollama's /api/chat with the given messages.

    Args:
        model: Model name
        system: System prompt
        user_message: User's message with context
        base_url: Ollama base URL
        temperature: LLM temperature

    Returns:
        Response text
    """
    url = f"{base_url}/api/chat"
    payload = json.dumps({
        "model": model,
        "messages": [
            {"role": "system", "content": system},
            {"role": "user", "content": user_message},
        ],
        "stream": False,
        "options": {
            "temperature": temperature,
            "num_ctx": 8192,
        },
    }).encode("utf-8")

    for attempt in range(3):
        try:
            req = Request(url, data=payload, headers={"Content-Type": "application/json"})
            resp = urlopen(req, timeout=120)

            # Handle streaming response (collect all chunks for non-streaming)
            result = json.loads(resp.read().decode("utf-8"))
            if "message" in result and "content" in result["message"]:
                return result["message"]["content"]
            else:
                return str(result)

        except URLError as e:
            if attempt < 2:
                wait = 1.0 * (attempt + 1)
                logger.warning(f"LLM call attempt {attempt + 1} failed: {e}. Retrying in {wait}s...")
                time.sleep(wait)
            else:
                raise RuntimeError(f"Cannot reach Ollama: {e}")
        except json.JSONDecodeError as e:
            raise RuntimeError(f"Invalid JSON from Ollama: {e}")

    return ""
