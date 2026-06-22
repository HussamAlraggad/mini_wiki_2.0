/** @jsxImportSource solid-js */
import { theme } from "../styles/theme"

type StatusBarProps = {
  model: string
  status: string
  ragReady: boolean
  messageCount: number
  width: () => number
}

export function StatusBar(props: StatusBarProps) {
  const modelLabel = props.model || "no model"
  const ragIcon = props.ragReady ? "RAG" : "KB"
  const ragColor = props.ragReady ? theme.success : theme.textMuted
  const statusColor = props.status === "Ready" ? theme.textMuted : theme.accentLight

  return (
    <box
      width={props.width()}
      height={1}
      backgroundColor={theme.bg}
    >
      <text color={theme.textMuted} width="100%">
        {"  " + modelLabel + "  |  "}
        <text color={ragColor}>{ragIcon}</text>
        {"  |  " + props.messageCount + " msgs  |  "}
        <text color={statusColor}>{props.status}</text>
      </text>
    </box>
  )
}
