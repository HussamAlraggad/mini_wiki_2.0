/** @jsxImportSource solid-js */
import { createMemo, For } from "solid-js"
import { theme } from "../styles/theme"

export type Suggestion = {
  name: string
  description: string
}

type AutocompleteProps = {
  input: () => string
  suggestions: () => Suggestion[]
  onSelect: (name: string) => void
}

export function Autocomplete(props: AutocompleteProps) {
  const isActive = () => props.input().startsWith("/") && props.input().length > 1

  const filtered = createMemo(() => {
    if (!isActive()) return []
    const query = props.input().slice(1).toLowerCase()
    return props.suggestions().filter(
      (s) => s.name.toLowerCase().startsWith(query)
    ).slice(0, 10)
  })

  return (
    <>
      {filtered().length > 0 && (
        <box
          width={40}
          height={Math.min(filtered().length + 1, 11)}
          backgroundColor={theme.bgLighter}
          borderStyle="single"
          borderColor={theme.accent}
        >
          <For each={filtered()}>
            {(s) => (
              <box width="100%" height={1} paddingX={1}>
                <text bold color={theme.accentLight} width={12}>
                  {"/" + s.name}
                </text>
                <text color={theme.textMuted} width={28}>
                  {s.description.slice(0, 27)}
                </text>
              </box>
            )}
          </For>
        </box>
      )}
    </>
  )
}
