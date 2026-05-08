"""
Agentic Ranker: Uses an LLM to write and execute Pandas filter code for dataset ranking.

Instead of scoring every row via LLM (slow, O(n) API calls), this module:
1. Loads the dataset into a Pandas DataFrame
2. Sends the schema + sample rows + research topic to a coder LLM
3. LLM generates a Python function that filters/scored the data
4. Executes the function in a sandboxed environment (pandas/numpy/json only)
5. Returns results as JSON (fast, O(1) API calls)

Supported formats: CSV, JSONL (auto-detected by extension).
"""

import json
import logging
import os
import time

logger = logging.getLogger(__name__)

# Sandbox: only modules allowed in generated code
SAFE_MODULES = {"pd": None, "np": None, "json": json}


def _load_dataframe(path: str):
    """Load a dataset into a Pandas DataFrame, auto-detecting format by extension."""
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
        raise ValueError(f"Unsupported format: {ext}. Supported: CSV, JSONL, JSON, XLSX, TSV")


def _call_llm_for_code(model: str, schema: dict, sample_rows: list, topic: str, ollama_base: str) -> str:
    """Ask the LLM to generate a Pandas filter function for the given schema and topic.

    Args:
        model: LLM model name (e.g. 'qwen2.5-coder:7b')
        schema: dict of {column_name: dtype_string}
        sample_rows: list of dicts (first 3 rows)
        topic: research topic to filter by
        ollama_base: Ollama API base URL

    Returns:
        Raw Python code string from the LLM (may contain markdown fences).
    """
    from rag_worker.querier import call_llm

    system_prompt = (
        "You are an expert Data Scientist. You write flawless Python Pandas code. "
        "Never explain the code. Never use markdown formatting. ONLY output raw Python code."
    )

    user_prompt = f"""
I have a Pandas DataFrame named `df`.
Schema: {json.dumps(schema, indent=2)}
Sample Data: {json.dumps(sample_rows, indent=2, default=str)}

Task: Write a Python function named `filter_data(df)` that returns a new DataFrame containing
ONLY the rows that are highly relevant to the research topic: "{topic}".

Requirements:
- Include a new column called "relevance_score" (integer 1 to 100) based on your logic.
- The function must only use pandas (pd) and numpy (np). No other imports.
- Do NOT use os, sys, subprocess, or any file I/O.
- Return ONLY the filtered DataFrame, do not print anything.
- Use vectorized Pandas operations (no for loops) for performance.
"""

    logger.warning(f"Asking {model} to write Pandas script for topic: {topic}")
    generated_code = call_llm(
        model=model,
        system=system_prompt,
        user_message=user_prompt,
        base_url=ollama_base,
        temperature=0.2,
    )

    # Strip markdown code fences if the LLM disobeys
    generated_code = generated_code.replace("```python", "").replace("```", "").strip()
    return generated_code


def agentic_rank(path: str, topic: str, llm_model: str, ollama_base: str) -> dict:
    """Rank a dataset by having an LLM write and execute a Pandas filter script.

    Args:
        path: Path to the dataset file (CSV, JSONL, XLSX, TSV)
        topic: Research topic to rank by
        llm_model: LLM model for code generation (e.g. 'qwen2.5-coder:7b')
        ollama_base: Ollama API base URL

    Returns:
        dict with keys:
            - type: "rank_done" on success, "error" on failure
            - rows_kept: number of rows after filtering
            - total_rows: total rows in original dataset
            - data: list of dicts (filtered rows with relevance_score)
            - message: status message or error description
    """
    start_time = time.time()

    try:
        # 1. Load the dataset
        df = _load_dataframe(path)
        total_rows = len(df)
        logger.warning(f"Loaded {total_rows} rows from {path}")

        # 2. Extract schema + sample (Do NOT send the whole file to the LLM)
        schema = {col: str(dtype) for col, dtype in df.dtypes.items()}
        sample_rows = df.head(3).to_dict(orient="records")

        # 3. Get the filter code from the LLM
        generated_code = _call_llm_for_code(llm_model, schema, sample_rows, topic, ollama_base)
        logger.warning(f"Generated code length: {len(generated_code)} chars")

        # 4. Safe execution: only pandas/numpy available
        local_env = dict(SAFE_MODULES)  # copy sandbox modules
        exec(generated_code, local_env)

        filter_function = local_env.get("filter_data")
        if not filter_function:
            raise ValueError(
                "The LLM did not generate a 'filter_data' function. "
                "Generated code:\n" + generated_code[:500]
            )

        # 5. Execute on the full dataset (takes ~0.1 seconds for 100K rows)
        filtered_df = filter_function(df)
        rows_kept = len(filtered_df)
        elapsed = time.time() - start_time

        # 6. Ensure relevance_score column exists
        if "relevance_score" not in filtered_df.columns:
            # Try numeric index
            filtered_df = filtered_df.reset_index()
            if "relevance_score" not in filtered_df.columns:
                # Add a default score
                filtered_df["relevance_score"] = 100

        # 7. Sort by relevance_score descending
        filtered_df = filtered_df.sort_values("relevance_score", ascending=False)

        # 8. Convert to JSON for the Go TUI
        result_data = json.loads(filtered_df.to_json(orient="records", default_handler=str))

        return {
            "type": "rank_done",
            "rows_kept": rows_kept,
            "total_rows": total_rows,
            "data": result_data,
            "message": f"Ranked in {elapsed:.1f}s: {total_rows} rows → {rows_kept} kept ({(rows_kept/total_rows*100) if total_rows else 0:.0f}%)",
        }

    except Exception as e:
        logger.exception("Agentic Rank Failed")
        return {
            "type": "error",
            "message": f"Agentic rank failed: {str(e)}",
            "rows_kept": 0,
            "total_rows": 0,
            "data": [],
        }
