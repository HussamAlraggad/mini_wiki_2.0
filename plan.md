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
* **Phase 1: Foundation & LLM Integration**
    * Set up Go environment and TUI framework (e.g., Bubbletea or tview).
    * Implement connection to local model runner (e.g., Ollama API).
    * Build the basic chat interface and model switching/fallback mechanism.
* **Phase 2: File System & Data Ingestion**
    * Implement cross-directory execution logic.
    * Build the CSV parsing and chunking engine.
    * Integrate the persistent vector database / knowledge base for dataset memory.
* **Phase 3: Web Fetching & Output Generation**
    * Develop the web scraping module for external links.
    * Implement tabular export functionality (to `.xlsx`).
* **Phase 4: Refinement**
    * Polish UI/UX (commands, shortcuts).
    * Testing against large CSV files to ensure stability.

## 7. Next Steps for the User
1.  Review this document and append any specific dataset requirements or constraints provided by the professor.
2.  Provide this `plan.md` to the coding assistant to begin scaffolding the Go TUI application.
