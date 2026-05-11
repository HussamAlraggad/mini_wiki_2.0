"""
Deep Reader: Reads each chunk like a human researcher would.

Instead of storing raw text, this module uses an LLM to "read and understand"
each chunk, producing a detailed explanation. Both the original chunk and
the AI's understanding are stored for searching.

Flow:
  chunk text → gemma4 reads it like a human → detailed explanation →
  both original + explanation are embedded and stored in ChromaDB
"""

import logging

logger = logging.getLogger(__name__)


def deep_read(chunk_text: str, llm_model: str, ollama_base: str) -> str:
    """Read a chunk like a human and return a detailed understanding.

    The LLM explains what the chunk means, what it implies, any patterns,
    connections, or important details — as if a researcher took notes on it.

    Args:
        chunk_text: The raw text chunk to analyze
        llm_model: The LLM model for deep reading (default: gemma4:e4b)
        ollama_base: Ollama API base URL

    Returns:
        A detailed textual "understanding" / analysis of the chunk
    """
    from rag_worker.querier import call_llm

    system_prompt = (
        "You are a brilliant, thorough researcher reading a document. "
        "Read the following text carefully, like a human expert would. "
        "Then explain in vivid detail: what this text means, its implications, "
        "any patterns you notice, connections to broader concepts, "
        "and why this information matters. "
        "Write your analysis as detailed notes — like a researcher annotating "
        "a primary source. Be specific, reference actual content from the text, "
        "and think deeply about what it reveals."
    )

    user_prompt = f"""
Please read this text and provide your detailed understanding and analysis:

--- BEGIN TEXT ---
{chunk_text}
--- END TEXT ---

Your analysis (be thorough and specific):
"""

    understanding = call_llm(
        model=llm_model,
        system=system_prompt,
        user_message=user_prompt,
        base_url=ollama_base,
        temperature=0.3,
    )

    return understanding
