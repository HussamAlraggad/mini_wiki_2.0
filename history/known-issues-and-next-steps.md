# Known Issues & Next Steps

> Generated: 2026-06-23
> Session paused: Bubbletea stripped, OpenCode-style TUI in progress

---

## 1. Critical: TUI Shows Blank Screen

### Symptom
Running `wiki` enters alt-screen mode (terminal goes blank) but no UI elements are drawn. The debug log shows "Render completed" — SolidJS components mount successfully, but nothing renders on screen.

### Root Cause
`requestAnimationFrame` is **undefined** in bun-compiled binaries (`bun build --compile --target=bun`).

OpenTUI's internal render loop uses `requestAnimationFrame` to schedule frame draws. Without it, the loop never executes and nothing is flushed to the terminal.

### Current Fix Attempt
In `wiki-tui/src/index.tsx` (lines 5-12):
```typescript
if (typeof globalThis.requestAnimationFrame === "undefined") {
  globalThis.requestAnimationFrame = (cb: (ts: number) => void): number => {
    return +setTimeout(() => cb(Date.now()), 16) // ~60fps
  }
}
if (typeof globalThis.cancelAnimationFrame === "undefined") {
  globalThis.cancelAnimationFrame = (id: number) => clearTimeout(id)
}
```

**Status**: Added but NOT verified. Build the TUI binary and test.

### How to test the fix
```bash
cd wiki-tui
bun build --compile --target=bun --outfile=dist/wiki-tui ./src/index.tsx
timeout 3 ./dist/wiki-tui   # If you see "mini-wiki" text, it works
```

### If the polyfill doesn't work
The opencode fork handles rendering differently. Study these files:

| File | What to look for |
|---|---|
| `packages/tui/src/app.tsx` | How `createCliRenderer()` and `render()` are called |
| `packages/tui/package.json` | Build scripts and dependencies |
| `packages/console/app/` | Alternative TUI entry point |
| Root `package.json` scripts | `dev:console` shows the run command |

The opencode TUI uses **Vite + `vite-plugin-solid`** for the JSX transform, not `bun build --compile` directly. Their entry point runs with:
```bash
bun run --cwd packages/console/app dev
```

Possible alternative approach: bundle with Vite + SolidJS plugin, then use `bun build --compile` on the Vite output.

---

## 2. Known Workarounds Already in Place

| Issue | Fix | File |
|---|---|---|
| `fetch()` can't reach localhost in compiled binary | Raw TCP `Bun.connect()` for health check | `wiki-tui/src/index.tsx` |
| `React.createElement` not defined (bun uses React JSX) | `globalThis.React` polyfill | `wiki-tui/src/index.tsx` |
| `solid-js/h` doesn't export `h` in v1.9.x | Removed direct `h` import, use React-style JSX | `wiki-tui/src/index.tsx` |

---

## 3. What Was Built (Working)

| Component | Status | Location |
|---|---|---|
| Go HTTP/SSE server | Verified | `internal/server/` |
| Tool registry (11 tools) | Verified | `internal/tool/` |
| Config (global + project) | Verified | `internal/config/` |
| Sessions (persist to JSON) | Verified | `internal/server/session.go` |
| RAG Python worker | Verified | `rag_worker/` |
| One-binary embed | Verified (128MB) | `//go:embed` + build tags |
| TUI scaffolding | Compiles, mounts | `wiki-tui/` |

### Commands
```bash
bash scripts/build-tui.sh          # Full build (TUI + Go embed)
wiki                                # Launch TUI (default)
wiki --serve                        # HTTP server only (headless)
wiki --no-start                     # Skip Ollama auto-start
```

### API Endpoints (when running --serve)
```
GET  /health
GET  /api/status
GET  /api/config
GET  /api/events        (SSE stream)
GET  /api/tools
GET  /api/sessions
POST /api/sessions       {title?}
POST /api/chat           {text, context?}
POST /api/query          {text, top_k?, deep?}
POST /api/ingest         {path, deep?}
POST /api/rank           {topic}
POST /api/model          {model}
GET  /api/models
```

---

## 4. How to Continue

1. **Fix the blank screen** — verify or replace the `requestAnimationFrame` polyfill
2. **Study opencode's TUI build** — specifically how they handle the SolidJS render loop
3. **Polish the TUI** once rendering works — autocomplete, command palette, error toasts
4. **Test all commands** — /help, /ingest, /query, /rank, /model, /models, /clear, /exit
5. **Write tests** — `internal/server/`, `internal/tool/`, `internal/config/` have no tests yet

---

## 5. Key Files Reference

```
wiki-tui/src/index.tsx         Entry point (rAF polyfill, health check, render)
wiki-tui/src/app.tsx           App component (commands, SSE, state)
wiki-tui/src/routes/session.tsx  Chat view (messages, sidebar, autocomplete)
wiki-tui/src/routes/home.tsx     Welcome screen
wiki-tui/src/client/api.ts       API client (HTTP + SSE)
wiki-tui/src/client/http.ts      Raw TCP HTTP client (for compiled binary)
internal/server/server.go        HTTP server setup
internal/server/handlers.go      API handlers
internal/server/session.go       Session persistence
internal/tool/tool.go            Tool type + registry
internal/tool/tools.go           11 default tools
main.go                          Entry point (server + TUI spawn)
```
