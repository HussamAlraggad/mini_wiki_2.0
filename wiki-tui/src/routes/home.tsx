/** @jsxImportSource solid-js */
import { useTerminalDimensions } from "@opentui/solid"
import { PromptInput } from "../components/input"
import { theme } from "../styles/theme"

type WelcomeScreenProps = {
  onSend: (text: string) => void
  input: () => string
  setInput: (val: string) => void
}

export function WelcomeScreen(props: WelcomeScreenProps) {
  const dims = useTerminalDimensions()
  const inputWidth = () => Math.min(60, Math.floor(dims().width * 0.6))

  return (
    <box
      width={dims().width}
      height={dims().height}
      backgroundColor={theme.bg}
      flexDirection="column"
      alignItems="center"
      justifyContent="center"
    >
      {/* Title with background box */}
      <box
        backgroundColor={theme.bgLighter}
        paddingX={4}
        paddingY={1}
        borderStyle="single"
        borderColor={theme.accent}
      >
        <text bold color={theme.accent} align="center" width={inputWidth()}>
          mini-wiki
        </text>
      </box>

      <box height={2} />

      {/* Subtitle */}
      <text color={theme.textMuted} align="center" width={inputWidth()}>
        Local AI Research Assistant
      </text>

      <box height={1} />

      {/* Model and RAG status */}
      <text color={theme.textMuted} align="center" width={inputWidth()}>
        Powered by Ollama | RAG-enabled
      </text>

      <box height={2} />

      {/* Help hint */}
      <text color={theme.border} align="center" width={inputWidth()}>
        Type a question or /help for commands
      </text>

      <box height={3} />

      {/* Input */}
      <box
        backgroundColor={theme.bgLighter}
        borderStyle="single"
        borderColor={theme.border}
        paddingX={1}
        paddingY={0}
      >
        <PromptInput
          value={props.input}
          onChange={props.setInput}
          onSubmit={props.onSend}
          placeholder="Type a message..."
          width={inputWidth()}
        />
      </box>
    </box>
  )
}
