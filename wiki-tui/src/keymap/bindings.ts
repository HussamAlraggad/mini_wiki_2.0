/**
 * Keyboard bindings for wiki-tui.
 * 
 * These are handled at the App level via OpenTUI's onKeyPress events.
 * We keep them centralized here for maintainability.
 */

export type Binding = {
  keys: string[]
  description: string
  action: () => void
}

export type BindingGroup = {
  name: string
  bindings: Binding[]
}

export const DEFAULT_BINDINGS: BindingGroup[] = [
  {
    name: "Input",
    bindings: [
      { keys: ["Enter"], description: "Submit message", action: () => {} },
      { keys: ["Alt+Enter"], description: "Insert newline", action: () => {} },
    ],
  },
  {
    name: "Navigation",
    bindings: [
      { keys: ["Tab"], description: "Cycle focus", action: () => {} },
      { keys: ["Ctrl+P"], description: "Command palette", action: () => {} },
    ],
  },
  {
    name: "Application",
    bindings: [
      { keys: ["Ctrl+C"], description: "Exit", action: () => {} },
      { keys: ["Ctrl+L"], description: "Clear conversation", action: () => {} },
    ],
  },
]
