"""
Querier module: embeds a query, searches the vector DB, retrieves context,
optionally reranks results, and sends to LLM for answer with sources.

Enhancements:
- Dual query rewriting: rewrite query for better retrieval while keeping
  original for LLM context (Phase 5)
- Built-in reranking: re-scores chunks with a cross-encoder for precision
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
from rag_worker.reranker import Reranker

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
    top_k: int = 10,
    system_prompt: str = None,
    ollama_base: str = "http://127.0.0.1:11434",
    deep: bool = False,
    deep_model: str = "gemma4:e4b",
    reranker: Optional[Reranker] = None,
    rewrite_model: Optional[str] = None,
) -> QueryResult:
    """Run a RAG query: embed -> search -> retrieve -> [rerank] -> [deep read] -> LLM -> answer.

    The pipeline:
    1. Optionally rewrite the query for better retrieval (keeps original for LLM)
    2. Embed the (rewritten) query
    3. Search vector DB (fetch more candidates when reranker is active)
    4. Rerank results with cross-encoder (if available)
    5. Optionally deep-read top chunks
    6. Send context + original question to LLM

    Args:
        question: User's question
        embedder: Embedder instance
        vector_db: VectorDB instance
        llm_model: LLM model for answering
        top_k: Number of chunks to retrieve (increased when reranker is active)
        system_prompt: Override default system prompt
        ollama_base: Ollama base URL
        deep: If True, use on-demand deep reading of retrieved chunks
        deep_model: LLM model to use for deep reading
        reranker: Optional Reranker instance for cross-encoder re-scoring
        rewrite_model: Optional model name for query rewriting (uses llm_model if not set)

    Returns:
        QueryResult with answer and sources
    """
    # 1. Optionally rewrite the query for better retrieval
    search_query = question
    if reranker is not None and reranker.available:
        # When reranker is available, fetch more candidates for better coverage
        search_top_k = max(top_k * 2, 10)
        final_top_k = top_k
    else:
        search_top_k = top_k
        final_top_k = top_k

    if rewrite_model or llm_model:
        try:
            search_query = _rewrite_query(question, rewrite_model or llm_model, ollama_base)
            if search_query and search_query != question:
                logger.info(f"Query rewritten for retrieval: {search_query}")
        except Exception as e:
            logger.warning(f"Query rewriting failed: {e}. Using original query.")
            search_query = question

    # 2. Embed the search query
    query_embedding = embedder.embed_single(search_query)
    if not query_embedding:
        return QueryResult(answer="Failed to embed query. Is Ollama running?", model=llm_model)

    # 3. Search vector DB (fetch extra candidates for reranker)
    results = vector_db.search(query_embedding, top_k=search_top_k)
    if not results:
        return QueryResult(
            answer="No relevant documents found in the knowledge base. Try ingesting files first with /ingest.",
            model=llm_model,
        )

    # 4. Rerank results with cross-encoder (if available)
    if reranker is not None and reranker.available:
        logger.info(f"Reranking {len(results)} chunks...")
        results = reranker.rerank(search_query, results, top_k=final_top_k)
    else:
        results = results[:final_top_k]

    # 5. Retrieve chunks and optionally do deep reading
    sources = []
    context_parts = []

    # Limit deep reading to top 3 chunks to keep latency reasonable
    deep_limit = min(3, len(results)) if deep else 0

    for i, r in enumerate(results):
        source = r.get("metadata", {}).get("source", "unknown")
        score = r.get("score", 0)
        text = r.get("text", "")

        display_text = text[:300] + "..." if len(text) > 300 else text
        source_entry = {
            "file": source,
            "score": round(score, 3),
            "text": display_text,
        }
        # Add rerank score if available
        if "rerank_score" in r:
            source_entry["rerank_score"] = r["rerank_score"]
        sources.append(source_entry)

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

    # 6. Build system prompt with both original and rewritten query context
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

    # Build the user message with both original question and rewritten context
    user_message_parts = [f"Question: {question}"]
    if search_query != question:
        user_message_parts.append(f"(Search also performed for: {search_query})")
    user_message_parts.append(f"\nContext:\n{context}")
    user_message = "\n\n".join(user_message_parts)

    # 7. Call LLM with context
    answer = call_llm(
        model=llm_model,
        system=system_prompt,
        user_message=user_message,
        base_url=ollama_base,
    )

    return QueryResult(answer=answer, sources=sources, model=llm_model)


def _rewrite_query(question: str, model: str, ollama_base: str) -> str:
    """Rewrite the user's question for better retrieval.

    Uses the LLM to generate a search-optimized version of the query
    while preserving the original for the final answer context.

    Args:
        question: Original user question
        model: LLM model for rewriting
        ollama_base: Ollama base URL

    Returns:
        Rewritten query (or original if rewriting fails)
    """
    prompt = (
        "Rewrite the following question to be more effective for searching "
        "a knowledge base. Keep it concise and focused on key terms. "
        "Respond with ONLY the rewritten question, no explanation.\n\n"
        f"Original: {question}\n\nRewritten:"
    )

    url = f"{ollama_base}/api/generate"
    payload = json.dumps({
        "model": model,
        "prompt": prompt,
        "stream": False,
        "options": {
            "temperature": 0.3,
            "num_predict": 200,
        },
    }).encode("utf-8")

    req = Request(url, data=payload, headers={"Content-Type": "application/json"})
    resp = urlopen(req, timeout=30)
    result = json.loads(resp.read().decode("utf-8"))

    rewritten = result.get("response", "").strip()
    if rewritten and len(rewritten) > 5 and rewritten != question:
        return rewritten
    return question


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
