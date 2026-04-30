# mini_wiki_2.0 — Standalone TUI AI Research Assistant

> An intelligent co-researcher powered entirely by local AI models.

---

## Table of Contents

- [Overview](#overview)
- [Tech Stack](#tech-stack)
- [Local AI & LLM Engine](#local-ai--llm-engine)
- [Features](#features)
  - [Data Ingestion & Analysis](#data-ingestion--analysis)
  - [Conversational Chat Interface](#conversational-chat-interface)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Installation](#installation)
  - [Running the Tool](#running-the-tool)
- [Usage](#usage)
  - [Chat Commands](#chat-commands)
- [Roadmap](#roadmap)
- [Contributing](#contributing)
- [License](#license)

---

## Overview

`mini_wiki_2.0` is a robust, standalone Terminal User Interface (TUI) tool built from scratch in Go. It acts as a "co-researcher" and intelligent assistant for Master's-level research in Software Engineering — specifically Software Requirements Engineering and Software Quality Engineering.

The tool runs **100% locally**: no cloud APIs, no data leaving your machine.

---

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go (Golang) |
| Interface | TUI (Terminal User Interface) |
| TUI Framework | [Bubbletea](https://github.com/charmbracelet/bubbletea) / [tview](https://github.com/rivo/tview) *(TBD)* |
| LLM Backend | [Ollama](https://ollama.com/) (local model runner) |
| Knowledge Base | Local vector database *(TBD)* |

The compiled binary is **directory-agnostic** — run it from any directory and it automatically treats the current working directory as the root for that research session.

---

## Local AI & LLM Engine

- **Strictly local:** No cloud API calls. Full privacy for your research data.
- **Model size cap:** ≤ 10 Billion parameters to run on 8 GB VRAM (RTX 4060 / RTX 5060).
- **Recommended models:**
  - `llama3:8b` — General reasoning and conversational tasks.
  - `deepseek-coder` / `codeqwen` — Software engineering, logic, and data analysis.
  - `gemma:4b` — Lightweight, fast alternative.
- **Model switching:** Switch models mid-session without losing context.
  ```
  /models
  ```
- **Future:** Multi-agent mode — prompt several local models simultaneously for diverse analytical perspectives.

---

## Features

### Data Ingestion & Analysis

- **Automated context loading** — scans the execution directory for relevant files on startup.
- **CSV deep analysis** — ingest large datasets (scaling to millions of rows); tasks include filtering, cleaning, anomaly detection, and requirement validation.
- **Persistent knowledge base** — dataset is processed once and stored locally so subsequent sessions load instantly.
- **Data export** — export cleaned/analyzed data to `.xlsx` or other tabular formats directly from the TUI.

### Conversational Chat Interface

- Continuous dialogue about the loaded dataset and research topic.
- **`/` commands** for system actions (e.g., `/models`).
- **`@` / `#` references** to specific files in the working directory.
- **URL ingestion** — paste any URL into the chat; the tool scrapes the page, pulls the content into the session, and synthesizes it with local data.

---

## Getting Started

### Prerequisites

- Go `1.21+`
- [Ollama](https://ollama.com/) installed and running locally
- At least one supported model pulled (e.g., `ollama pull llama3`)

### Installation

```bash
# Clone the repository
git clone https://github.com/HussamAlraggad/mini_wiki_2.0.git
cd mini_wiki_2.0

# Build the binary
go build -o mini-wiki .
```

### Running the Tool

```bash
# Navigate to your research directory
cd /path/to/your/research/

# Launch the tool
/path/to/mini-wiki
```

---

## Usage

### Chat Commands

| Command | Description |
|---|---|
| `/models` | List available local models and switch the active one |
| `@<filename>` | Reference a specific file from the working directory |
| `#<topic>` | Tag a topic for contextual focus *(planned)* |

---

## Roadmap

- [x] **Phase 1 — Foundation & LLM Integration**
  - Go environment & TUI framework setup
  - Ollama API connection
  - Basic chat interface & model switching/fallback
- [ ] **Phase 2 — File System & Data Ingestion**
  - Cross-directory execution logic
  - CSV parsing & chunking engine
  - Persistent vector database / knowledge base
- [ ] **Phase 3 — Web Fetching & Output Generation**
  - Web scraping module for external URLs
  - Tabular export (`.xlsx`)
- [ ] **Phase 4 — Refinement**
  - UI/UX polish (commands, shortcuts)
  - Stability testing against large CSV files

---

## Contributing

Contributions, issues, and feature requests are welcome. Please open an issue first to discuss what you would like to change.

---

## License

*License TBD.*
