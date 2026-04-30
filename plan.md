# Project Plan: Standalone TUI AI Research Assistant

## 1. Project Overview
**Objective:** Develop a robust, standalone Terminal User Interface (TUI) tool powered entirely by local AI models. The tool will serve as a "co-researcher" and intelligent assistant for Master's-level research in Software Engineering (specifically Software Requirements Engineering and Software Quality Engineering).
**Execution Strategy:** Build entirely from scratch to avoid legacy technical debt, utilizing lessons learned from previous iterations.

## 2. Core Architecture & Tech Stack
* **Primary Codebase:** Go (Golang). Selected for its high performance, excellent concurrency handling, and ability to compile into a single, standalone binary.
* **Interface:** Terminal User Interface (TUI).
* **Execution Environment:** Directory-agnostic. The compiled binary must be executable from *any* arbitrary directory on the machine (e.g., `cd /path/to/research/dir && my-tui-tool`). It will automatically treat the current working directory as the root for that specific research session.
* **Hardware Constraints:** Optimized for machines with 8GB VRAM GPUs (e.g., RTX 4060 and RTX 5060). 

## 3. Local AI & LLM Engine
* **Strictly Local Operation:** No reliance on cloud APIs (e.g., OpenAI). Complete privacy for research data.
* **Model Parameters:** Capped at **10 Billion parameters** or less to run efficiently on 8GB VRAM without Out-Of-Memory (OOM) errors.
* **Recommended Models:**
    * *Llama 3 (8B)* - General reasoning and conversational capabilities.
    * *DeepSeek Coder / CodeQwen* - Highly tuned for software engineering, logic, and data analysis tasks.
    * *Gemma (4B)* - Lightweight, fast alternative.
* **Fallback Mechanism:** A built-in feature allowing the user to seamlessly switch models mid-session. If a model fails, hallucinates, or isn't suited for a specific analytical task, the TUI will permit a quick swap to an alternative local model (similar to Open-WebUI model selection).
    * This can be done manually by the user too, using this command:
    
        ```bash
        /models
        ```

* **Future Capability (Multi-Agent):** Architecture should allow future scalability to prompt multiple models simultaneously ("co-researchers") for diverse analytical perspectives, hardware permitting.

## 4. Data Ingestion & Analysis Features
* **Automated Context Loading:** The TUI will scan the execution directory for relevant files.
* **CSV Deep Analysis:**
    * Ability to ingest, digest, and learn from large datasets (e.g., CSV files located in the root directory).
    * Capable of handling substantial record counts (scaling up to millions of rows, processing once to build a persistent knowledge base).
    * Analytical tasks include: Filtering, cleaning, anomaly detection, and requirement validation.
* **Data Export:** Capability to export the cleaned and analyzed data into Excel (`.xlsx`) or other tabular formats directly from the TUI.
* **Persistent Knowledge Base:** Once a dataset is ingested (which may take several hours for massive sets), the knowledge is stored locally so the AI can reference it instantly in future sessions without re-processing.

## 5. Conversational & Chat Functionality
* **Interactive Discussion:** A chat interface allowing continuous dialogue regarding the dataset and research topic.
* **Advanced Chat Commands:** Support for advanced parsing (e.g., `/` commands for system actions, `@` or `#` to reference specific files within the working directory).
* **Internet Research & Link Fetching:** * User can paste external URLs into the chat.
    * The tool will autonomously navigate to the link, scrape/read the text content, and bring that information back into the TUI.
    * The AI will then synthesize the external web data with the local dataset to assist in the research.

## 6. Development Roadmap

### Phase 1: Foundation & LLM Integration (COMPLETE)
- Go module + Bubbletea TUI framework
- Ollama HTTP client with streaming, timeouts, SSRF-safe dial
- Model manager: switching, fallback chain, active model tracking
- Config persistence (~/.config/mini-wiki/config.yaml, 0600 perms)
- Conversation types (Thread, Message, truncation)
- Structured error types (16 Kind values with predicates)
- Ollama auto-start/stop lifecycle management
- Command auto-completion with Tab cycling

### Phase 2: File System & Data Ingestion (COMPLETE)
- Safe directory scanner (skip dotfiles, symlink checks, binary detection via magic bytes + null byte check)
- File type detection (22 types: code, markup, data, config)
- Streaming CSV parser with chunking, context cancellation, column type detection
- @file reference resolver with path traversal protection + size limits
- TUI commands: /scan, /files, /ingest, @filename auto-attach

### Phase 3: Web Fetching & Output Generation (COMPLETE)
- Web fetcher with SSRF protection (11 private CIDR blocks, DNS resolution check)
- HTML-to-text extraction via golang.org/x/net/html (safe parsing, no regex)
- .xlsx export with formula injection protection
- SQLite knowledge base with FTS5 full-text search
- TUI commands: /fetch, /export, /kbstatus, /kbquery

### Phase 4: Dual Knowledge Base System (COMPLETE)
- Project KB (per-directory SQLite in .wiki/kb.sqlite)
  - Tables: srs_runs, srs_requirements, srs_moscow, srs_dfd_components, srs_cspec, srs_documents, query_history, bookmarks, filter_states
- Tool Memory (global YAML in ~/.config/mini-wiki/memory/)
  - Files: skills.yaml, flaws.yaml, session.yaml
  - 13 pre-registered skills, flaws tracking with solutions
- TUI commands: /bookmark, /bookmarks, /history, /skills, /flaws

### Phase 5: SRS Generation Pipeline (COMPLETE)
- 5-stage pipeline: FR/NFR Extraction - MoSCoW Prioritization - DFD Generation - CSPEC Logic - SRS Formatting
- 5 Go text/template prompts ported from Python Jinja2 originals (IEEE 830 compliant)
- All results stored in Project KB for cross-session reference
- TUI command: /srs

### Phase 6: Remaining Features (PLANNED)
- Ranking Engine (relevance + importance hybrid scoring)
- Filter Engine (filter by score, date, tags, source)
- Batch Processor (process large datasets in chunks)
- Setup Wizard (/wizard command)
- Enhanced Export (JSON, Markdown, CSV, PDF)

## 7. Next Steps for the User
1.  Run `./wiki` and test the tool
2.  Try `/srs` to generate an SRS document from conversation data
3.  Report any issues via `/flaws` (logs to tool memory)

## My notes:
* There are some functionalities I need to add:
    * The must be capable of:
        * check the OS of the machine.
        * install/downloads the needed dependencies (make them active immediately).
