"""
RAG Worker main entry point.
Reads JSON commands from stdin, processes them, writes JSON responses to stdout.

Protocol:
  Input (stdin):   {"cmd": "ingest", "path": "...", "embed_model": "..."}
  Output (stdout): {"type": "done", ...}  or  {"type": "error", ...}

Commands:
  ingest <path>            — Ingest a file into the vector DB
  query <text> [top_k]    — RAG query
  status                   — Get index stats
  ping                     — Health check (returns pong)
  shutdown                 — Exit cleanly
"""

import json
import logging
import os
import sys
import traceback

# Debug: log which Python interpreter is running
import_path = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, import_path)

# Print diagnostics to stderr before imports so we can debug startup failures
import io
_diag = io.StringIO()
_diag.write(f"[RAG worker] sys.executable: {sys.executable}\n")
_diag.write(f"[RAG worker] sys.prefix: {sys.prefix}\n")
_diag.write(f"[RAG worker] import path: {import_path}\n")
_diag.write(f"[RAG worker] PYTHONPATH (first 3): {[p for p in sys.path[:3]]}\n")
sys.stderr.write(_diag.getvalue())
sys.stderr.flush()

from rag_worker.embedder import Embedder
from rag_worker.vectordb import VectorDB
from rag_worker.ingester import ingest_file, IngestionResult
from rag_worker.querier import query, QueryResult

logging.basicConfig(
    level=logging.WARNING,
    format="%(levelname)s: %(message)s",
    stream=sys.stderr,
)
logger = logging.getLogger(__name__)


def send_response(data: dict):
    """Write JSON response to stdout and flush."""
    print(json.dumps(data), flush=True)


def main():
    # Read config from environment
    wiki_dir = os.environ.get("WIKI_DIR", os.getcwd())
    embed_model = os.environ.get("WIKI_EMBED_MODEL", "nomic-embed-text")
    llm_model = os.environ.get("WIKI_LLM_MODEL", "qwen2.5-coder")
    ollama_url = os.environ.get("WIKI_OLLAMA_URL", "http://127.0.0.1:11434")

    rag_dir = os.path.join(wiki_dir, ".wiki", "rag")
    os.makedirs(rag_dir, exist_ok=True)

    # Initialize components
    vector_db = VectorDB(rag_dir)
    embedder = Embedder(model=embed_model, base_url=ollama_url)

    # Send ready signal
    send_response({"type": "ready", "embed_model": embed_model, "llm_model": llm_model, "rag_dir": rag_dir})

    # Main loop: read JSON commands from stdin
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue

        try:
            cmd = json.loads(line)
        except json.JSONDecodeError as e:
            send_response({"type": "error", "message": f"Invalid JSON: {e}"})
            continue

        cmd_type = cmd.get("cmd", "")

        try:
            if cmd_type == "ingest":
                path = cmd.get("path", "")
                if not path:
                    send_response({"type": "error", "message": "Missing 'path' field"})
                    continue

                if not os.path.exists(path):
                    send_response({"type": "error", "message": f"File not found: {path}"})
                    continue

                chunk_size = cmd.get("chunk_size", 800)
                overlap = cmd.get("overlap", 100)

                # Use specified model if provided, otherwise default
                current_embedder = embedder
                if "embed_model" in cmd and cmd["embed_model"]:
                    current_embedder = Embedder(model=cmd["embed_model"], base_url=ollama_url)

                result = ingest_file(path, current_embedder, vector_db, chunk_size, overlap)
                # Send progress messages during ingestion
                if result.progress:
                    for p in result.progress:
                        send_response({"type": "progress", "message": p})
                if result.error:
                    send_response({"type": "error", "message": result.error})
                else:
                    total = vector_db.count()
                    send_response({
                        "type": "done",
                        "path": result.path,
                        "chunks": result.chunks,
                        "total_chunks": total,
                    })

            elif cmd_type == "query":
                text = cmd.get("text", "")
                if not text:
                    send_response({"type": "error", "message": "Missing 'text' field"})
                    continue

                top_k = cmd.get("top_k", 5)
                current_llm = cmd.get("llm_model", llm_model)

                result = query(
                    question=text,
                    embedder=embedder,
                    vector_db=vector_db,
                    llm_model=current_llm,
                    top_k=top_k,
                    ollama_base=ollama_url,
                )
                send_response({
                    "type": "answer",
                    "answer": result.answer,
                    "sources": result.sources,
                    "model": result.model,
                })

            elif cmd_type == "status":
                total = vector_db.count()
                sources = vector_db.list_sources()
                send_response({
                    "type": "status",
                    "total_chunks": total,
                    "sources": sources,
                    "embed_model": embedder.model,
                    "rag_dir": rag_dir,
                })

            elif cmd_type == "ping":
                send_response({"type": "pong"})

            elif cmd_type == "shutdown":
                send_response({"type": "bye"})
                sys.exit(0)

            else:
                send_response({"type": "error", "message": f"Unknown command: {cmd_type}"})

        except Exception as e:
            logger.exception("Command failed")
            send_response({"type": "error", "message": str(e), "traceback": traceback.format_exc()})


if __name__ == "__main__":
    main()
