"""
Agentic Query: Answers dataset questions by generating and executing Pandas code.

Flow:
1. Load the dataset with Pandas
2. Extract schema + 3 sample rows
3. Send schema + question to a coder LLM
4. LLM writes a Pandas query function
5. Execute in sandbox (pandas/numpy/json only)
6. Return the result as structured JSON

Supports: CSV, JSONL, XLSX, TSV (auto-detected by extension).
"""

import json
import logging
import time

logger = logging.getLogger(__name__)

# Sandbox modules (same as agentic_ranker)
SAFE_MODULES = {}


def _load_dataframe(path: str):
    """Load a dataset into a Pandas DataFrame, auto-detecting format."""
    import os
    import pandas as pd
    SAFE_MODULES["pd"] = pd
    try:
        import numpy as np
        SAFE_MODULES["np"] = np
    except ImportError:
        pass

    ext = os.path.splitext(path)[1].lower()

    if ext == ".csv":
        return pd.read_csv(path)
    elif ext in (".jsonl", ".ndjson", ".jsonlines"):
        return pd.read_json(path, lines=True)
    elif ext == ".json":
        return pd.read_json(path)
    elif ext in (".xlsx", ".xls"):
        return pd.read_excel(path)
    elif ext == ".tsv":
        return pd.read_csv(path, sep="\t")
    else:
        raise ValueError(f"Unsupported format: {ext}")


def _ask_llm(model: str, schema: dict, sample_rows: list, question: str, ollama_base: str) -> str:
    """Ask the LLM to generate a Pandas function that answers the question."""
    from rag_worker.querier import call_llm

    system_prompt = (
        "You are an expert data scientist. You write precise Pandas code. "
        "Never explain. Never use markdown. Output ONLY raw Python code."
    )

    user_prompt = f"""
I have a Pandas DataFrame named `df`.
Schema: {json.dumps(schema, indent=2)}
Sample rows: {json.dumps(sample_rows, indent=2, default=str)}

Write a Python function named `answer_query(df)` that:
- Answers this question: "{question}"
- Returns a string with a clear, concise answer (1-3 sentences with specific numbers)
- Uses vectorized Pandas operations (no for loops)
- Only uses pandas (pd) and numpy (np) — no other imports
- Does NOT print anything — return the answer string

If the question cannot be answered from this data, return: "This dataset does not contain the information needed to answer that question. Available columns: {list(schema.keys())}"
"""

    generated_code = call_llm(
        model=model,
        system=system_prompt,
        user_message=user_prompt,
        base_url=ollama_base,
        temperature=0.2,
    )
    generated_code = generated_code.replace("```python", "").replace("```", "").strip()
    return generated_code


def agentic_query(path: str, question: str, llm_model: str, ollama_base: str) -> dict:
    """Answer a question about a dataset by generating and executing Pandas code.

    Args:
        path: Path to the dataset file
        question: Natural language question about the data
        llm_model: Model to use for code generation
        ollama_base: Ollama API base URL

    Returns:
        dict with keys:
            type: "query_answer" on success, "error" on failure
            answer: The answer text
            sql_equivalent: The generated code (for transparency)
    """
    start_time = time.time()
    import os

    try:
        # 1. Load dataset
        df = _load_dataframe(path)
        total_rows = len(df)

        # 2. Extract schema + sample
        schema = {col: str(dtype) for col, dtype in df.dtypes.items()}
        sample_rows = df.head(3).to_dict(orient="records")

        # 3. Get code from LLM
        generated_code = _ask_llm(llm_model, schema, sample_rows, question, ollama_base)

        # 4. Execute safely
        local_env = dict(SAFE_MODULES)
        exec(generated_code, local_env)

        answer_func = local_env.get("answer_query")
        if not answer_func:
            raise ValueError(f"LLM did not generate 'answer_query' function. Code:\n{generated_code[:500]}")

        # 5. Run on actual data
        answer = answer_func(df)
        elapsed = time.time() - start_time

        return {
            "type": "query_answer",
            "answer": str(answer),
            "code": generated_code,
            "rows_analyzed": total_rows,
            "elapsed_seconds": round(elapsed, 1),
        }

    except Exception as e:
        logger.exception("Agentic Query failed")
        return {
            "type": "error",
            "message": f"Agentic query failed: {str(e)}",
        }
