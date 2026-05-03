# AI Agent Operating Instructions

> This file is the **operating manual** for AI agents on this project.
> It defines how to read the project, how to work, and how to hand off.
>
> Three files form the handoff system:
>   - `AGENTS.md` (this file) -- operating rules
>   - `plan.md` -- the specification (what to build)
>   - `journal.md` -- the state (where we are, what broke)

---

## 1. Reading Order (MANDATORY)

Every agent **MUST** read these files in this exact order at the start of their session:

1. **`journal.md`** first -- understand the current state, last agent's handoff, known issues.
2. **`plan.md`** second -- understand what to build, the exact specs, and the inter-phase contracts.
3. **`AGENTS.md`** (this file) third -- understand the operating rules.

**Never skip any of these three files.** If you skip, you will make incorrect assumptions.

---

## 2. The Three Pillars of Handoff

### plan.md (The Spec)
- Describes what the tool does and how it works.
- Contains exact command syntax, behavior, edge cases, error messages.
- **Section 12 (Phase Interop Contracts)** defines the Go types and interfaces that all phases
  must share. If you are implementing a PLANNED phase, you MUST read section 12 first and
  conform your code to those contracts.
- If a detail is missing or ambiguous, do NOT guess -- stop and ask.

### journal.md (The State)
- Tells you where the project is right now: what phase, what's working, what's broken.
- The `Current Project State` table at the top is your starting context.
- The `Handoff Notes` section contains critical context from the previous agent.
- The `Known Issues & Workarounds` table documents every problem and its resolution.
- After your session, you will append a new entry at the top of the journal.

### AGENTS.md (The Rules -- this file)
- How to operate, what to read, what to write, how to hand off.

---

## 3. Workflow Rules

### 3.1 Before You Start
1. Read journal.md, plan.md, AGENTS.md (in that order).
2. Run `go build -o wiki .` to confirm the binary builds.
3. Run `go test ./...` to confirm tests are green.
4. Run `go vet ./...` to confirm no warnings.

### 3.2 While Working
- NEVER add features, commands, UI elements, or packages not documented in plan.md.
- NEVER use emojis, icons, bright colors, or animations (see plan.md section 4).
- NEVER modify code in COMPLETE phases unless explicitly asked.
- NEVER touch deprecated packages (webfetch, srs) -- they are kept for reference only.
- ALWAYS use the types and interfaces from plan.md section 12 (Phase Interop Contracts).
- ALWAYS write tests for new code (see plan.md section 16 for requirements).
- ALWAYS update `journal.md` before finishing your session.

### 3.3 When in Doubt
- If something in plan.md is ambiguous: **ask** (do not guess).
- If something in journal.md contradicts plan.md: **plan.md wins** -- flag the contradiction.
- If you break something: **document it** in the "What I struggled with / broke" section of your journal entry.

---

## 4. Phase Status Definitions

Each phase in the Development Roadmap (plan.md section 11) has one of these statuses:

| Status | Meaning |
|---|---|
| `COMPLETE` | Fully implemented, tested, and working. Do NOT modify without explicit request. |
| `PLANNED` | Not yet started. The spec exists in plan.md. The interop contracts are defined. |
| `INPROGRESS` | Currently being worked on. Check journal.md for who, what, and status. |

---

## 5. Handoff Protocol

Every agent **MUST** end their session by appending a new entry at the **top** of `journal.md`
(above the previous entries). The entry must follow this exact format:

```markdown
## <Date> -- <Phase Name> (<STATUS>)

### What was done
- bullet list of actual accomplishments

### Interface changes made
- list any changes to types, interfaces, or contracts defined in plan.md section 12
- if none, write: "None"

### What I struggled with / broke
- honest list: bugs introduced and fixed, design mistakes, edge cases discovered
- specific file/line references for anything tricky

### Test status
```
(output of `go test ./...`)
```

### Handoff to next agent
- what the next agent needs to know before they start
- any unfinished work, known limitations, design decisions
- exact files and line numbers for anything non-obvious
```

**Requirements:**
- Do NOT delete or modify historical journal entries (append only).
- The "What I struggled with / broke" section is mandatory -- even if it's embarrassing.
- The "Handoff to next agent" section is mandatory -- even if you think everything is obvious.
- If you found a workaround for a system issue, add it to the `Known Issues & Workarounds` table.

---

## 6. Pre-Commit Checklist (Before Marking a Phase COMPLETE)

1. [ ] `go build -o wiki .` succeeds with no errors
2. [ ] `go test ./...` passes all tests
3. [ ] `go vet ./...` produces no warnings
4. [ ] `journal.md` updated with your session entry (at the top)
5. [ ] `journal.md` handoff section written
6. [ ] Any new package directories added to `plan.md` section 14 (Project File Structure)
7. [ ] All new public types and functions have comments
