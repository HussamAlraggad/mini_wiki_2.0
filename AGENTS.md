# AGENTS.md

Operating manual for AI agents on **mini-wiki** — a local-only TUI AI research assistant (Go + Bubbletea + Ollama).

## Handoff docs (read in this order)

1. **`journal.md`** — current state, last session's handoff, known issues
2. **`plan.md`** — the spec (what to build, contracts, design rules). If journal.md contradicts, plan.md wins.
3. **`AGENTS.md`** (this file) — operating rules

## Commands

| What | How |
|---|---|
| Build | `go build -o wiki .` |
| All tests | `go test ./...` |
| Single pkg | `go test ./internal/ranking/...` |
| Verbose | `go test -v ./...` |
| Race detect | `go test -race ./...` |
| Vet | `go vet ./...` |
| Setup deps | `bash setup.sh` |
| Run | `./wiki` (or global `wiki`) |

No CI workflows, no Makefile, no `.golangci.yml`. Plain Go project.

## Project state

**All 7 phases are COMPLETE** (Foundation, File Ingestion, RAG KB, Ranking, Charts, Export, Wizard). The project is feature-complete per plan.md. Every phase entry in plan.md section 11 is marked COMPLETE.

Phases are defined in plan.md section 11 with interop contracts in section 12. Do NOT modify COMPLETE phases without explicit request.

## What to know before editing

- **Error types**: Use `wiki.New(kind, msg)` / `wiki.Wrap(kind, msg, cause)` from `internal/wiki/errors.go`. 26 Kind constants with predicates (`IsConnection`, `IsTimeout`, etc.). Do NOT create raw errors.
- **Shared data types**: `internal/dataset/` defines `Dataset`, `Row`, `Column`, `ColumnKind`. All packages must import these — never define your own row/column types.
- **AppState**: Bubbletea states in `internal/app/app.go` — `StateIdle`, `StateStreaming`, `StateSearching`, `StateRanking`, `StateCharting`, `StateExporting`, `StateIngesting`, `StateConfirming`.
- **Intent detection** for NL commands lives in `internal/app/intent.go`. Tools defined there (rank, chart, export, discard, dataset_info, ingest).
- **Python RAG worker** (`rag_worker/*.py`) is embedded via `//go:embed rag_worker/*.py rag_worker/*.txt` in `main.go`. Extracted to a temp dir at runtime. Protocol: JSON-over-stdin/stdout between Go and Python subprocess.
- **`.venv/`** is project-local (gitignored), symlinked to `~/.config/mini-wiki/.venv` for global access. Go binary checks `.venv/bin/python3` first, then system python3, then python.
- **Ollama**: hardcoded to `127.0.0.1:11434` (not `localhost` — DNS rebinding protection). Context timeouts: 30s chat, 5s ping. Auto-started unless `--no-start` flag.
- **Per-project state**: `$CWD/.wiki/`. Global config: `~/.config/mini-wiki/`.
- **Deprecated packages** (do NOT touch): `internal/srs/`, `internal/webfetch/`. Kept for reference only.
- **Known bug** (csvparser): `detectType` is dead code — `updateColumnTypes` skips it (line 293 `if columns[i].Type == ColumnString { continue }`). Type narrowing never executes.

## Document management

- **AI-generated planning documents** (e.g., `PLAN.md`, `ARCHITECTURE.md`) MUST be stored in the `history/` directory to keep the repo root clean. Only hand-maintained files like `journal.md` and `AGENTS.md` live at the root.

## Design rules (strict)

- **No emojis** anywhere — not in code, comments, UI, or commits
- **No icons** — no Unicode pictograms, no ASCII art except the welcome logo
- **Colors** — no restriction. Use any palette that serves the UI.
- **Loading animation**: Bubbletea `spinner.Dot` model during LLM stream, ingestion, or ops >1s. Other animations OK where they add value — no gratuitous or distracting motion.
- **5-container layout**: Header (D), Chat (A), Right Panel (C), Input (B), Footer (E). Responsive breakpoints at <80, 80-119, >=120 cols. Min terminal: 60x16.
- **Mouse**: Left-click disables mouse tracking (native text selection). Typing re-enables it.

## Mistake-driven rules

Every mistake or struggle discovered during a session must produce two artifacts:
1. **A rule** in this file (AGENTS.md) — a concise guardrail that prevents the same mistake or aids recovery if it repeats. Add it under the relevant section or as a new bullet.
2. **A log entry** — detail the mistake in journal.md under either:
   - "What I struggled with / broke" (session entry section)
   - "Known Issues & Workarounds" table (for unresolved or recurring problems)

This is recursive: if you forget to add the rule, that itself is a mistake and must generate another rule.

## Before you start

```bash
go build -o wiki .   # confirm build
go test ./...        # confirm all green
go vet ./...         # confirm no warnings
```

## Before marking a phase COMPLETE

1. `go build -o wiki .` succeeds
2. `go test ./...` passes
3. `go vet ./...` no warnings
4. Update `journal.md` with your session entry at the top
5. Add any new package dirs to plan.md section 14

## End-of-session handoff

Append a new entry at the **top** of `journal.md`. Format:

```markdown
## <Date> -- <Phase> (<STATUS>)

### What was done
- bullet list

### Interface changes made
- changes to plan.md section 12 types/interfaces, or "None"

### What I struggled with / broke
- honest, with file/line references for tricky parts

### Test status
```
go test ./... output
```

### Handoff to next agent
- what they need to know, unfinished work, exact file/line references
```

Do not delete or modify historical journal entries (append only).

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:7510c1e2 -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/SYNC_CONCEPTS.md for details and anti-patterns.

## Session Completion

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
<!-- END BEADS INTEGRATION -->
