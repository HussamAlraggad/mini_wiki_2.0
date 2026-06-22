/** @jsxImportSource solid-js */
import { For, Show } from "solid-js"
import { useTerminalDimensions } from "@opentui/solid"
import type { Message } from "../app"
import { PromptInput } from "../components/input"
import { Autocomplete, type Suggestion } from "../component/autocomplete"
import { theme } from "../styles/theme"

export type SourceInfo = {
  file: string
  score: number
  text?: string
}

type ChatPanelProps = {
  messages: Message[]
  streamingContent: string
  status: string
  onSend: (text: string) => void
  input: () => string
  setInput: (val: string) => void
  isWide: boolean
  isNarrow: boolean
  width: number
  height: number
  sources?: SourceInfo[]
  datasetInfo?: string
  toolSuggestions?: Suggestion[]
}

function UserMessage(props: { content: string }) {
  return (
    <box width="100%" paddingX={2} paddingY={1}>
      <text color={theme.accent} bold height={1}>
        {" You"}
      </text>
      <box
        width="100%"
        backgroundColor={theme.bgLighter}
        borderStyle="single"
        borderColor={theme.border}
        paddingX={1}
        paddingY={0}
      >
        <text color={theme.text} width="100%">
          {props.content}
        </text>
      </box>
    </box>
  )
}

function AssistantMessage(props: { content: string }) {
  return (
    <box width="100%" paddingX={2} paddingY={1}>
      <text color={theme.success} bold height={1}>
        {" AI"}
      </text>
      <box
        width="100%"
        backgroundColor={theme.bgDarker}
        borderStyle="single"
        borderColor={theme.border}
        paddingX={1}
        paddingY={0}
      >
        <markdown content={props.content} streaming={false} conceal color={theme.text} />
      </box>
    </box>
  )
}

function StreamingMessage(props: { content: string }) {
  return (
    <box width="100%" paddingX={2} paddingY={1}>
      <text color={theme.success} bold height={1}>
        {" AI (streaming)"}
      </text>
      <box
        width="100%"
        backgroundColor={theme.bgDarker}
        borderStyle="single"
        borderColor={theme.accentLight}
        paddingX={1}
        paddingY={0}
      >
        <markdown content={props.content} streaming conceal color={theme.text} />
      </box>
    </box>
  )
}

function SystemMessage(props: { content: string }) {
  return (
    <box width="100%" paddingX={2} paddingY={0}>
      <text color={theme.textMuted} italic width="100%">
        {" " + props.content}
      </text>
    </box>
  )
}

function SourcePanel(props: { sources: SourceInfo[] }) {
  return (
    <box width="100%" flexDirection="column" paddingX={1} paddingY={1}>
      <text color={theme.textMuted} bold height={1}>
        {" Sources"}
      </text>
      <box height={1} />
      <For each={props.sources.slice(0, 5)}>
        {(source) => (
          <box width="100%" height={2} flexDirection="column" paddingY={0}>
            <text color={theme.accentLight} width="100%">
              {source.file.length > 25 ? "..." + source.file.slice(-22) : source.file}
            </text>
            <text color={theme.textMuted} width="100%">
              {"  " + (source.score * 100).toFixed(0) + "% match"}
            </text>
          </box>
        )}
      </For>
    </box>
  )
}

export function ChatPanel(props: ChatPanelProps) {
  const dims = useTerminalDimensions()

  const headerHeight = 1
  const inputHeight = 4
  const chatHeight = () => dims().height - headerHeight - inputHeight
  const hasSidebar = () => props.isWide
  const sidebarWidth = 28
  const mainWidth = () => (hasSidebar() ? dims().width - sidebarWidth : dims().width)

  return (
    <box width={dims().width} height={dims().height} flexDirection="column" backgroundColor={theme.bgDarker}>
      {/* Header bar */}
      <box width="100%" height={headerHeight} backgroundColor={theme.bg} paddingX={1}>
        <text bold color={theme.accent} width="100%">
          {" mini-wiki"}
          <text color={theme.textMuted}> {"  |  " + props.status}</text>
        </text>
      </box>

      {/* Main content */}
      <box width="100%" height={chatHeight()} flexDirection="row">
        {/* Chat area */}
        <scrollbox
          width={mainWidth()}
          height="100%"
          stickyScroll
          stickyStart="bottom"
          backgroundColor={theme.bgDarker}
        >
          <For each={props.messages}>
            {(msg) => (
              <>
                {msg.role === "user" && <UserMessage content={msg.content} />}
                {msg.role === "assistant" && <AssistantMessage content={msg.content} />}
                {msg.role === "system" && <SystemMessage content={msg.content} />}
              </>
            )}
          </For>
          {props.streamingContent && <StreamingMessage content={props.streamingContent} />}
        </scrollbox>

        {/* Sidebar */}
        <Show when={hasSidebar()}>
          <box
            width={sidebarWidth}
            height="100%"
            backgroundColor={theme.bg}
            borderStyle="single"
            borderColor={theme.border}
            flexDirection="column"
          >
            {/* Dataset info */}
            <Show when={props.datasetInfo}>
              <box width="100%" paddingX={1} paddingY={1} flexDirection="column">
                <text color={theme.textMuted} bold height={1}>
                  {" Dataset"}
                </text>
                <box height={1} />
                <text color={theme.text} width="100%">
                  {props.datasetInfo}
                </text>
              </box>
            </Show>

            {/* RAG sources */}
            <Show when={props.sources && props.sources.length > 0}>
              <box height={1} />
              <SourcePanel sources={props.sources || []} />
            </Show>

            {/* Keybindings hint */}
            <box width="100%" paddingX={1} flexDirection="column">
              <box height={1} />
              <text color={theme.border} bold height={1}>
                {" Keys"}
              </text>
              <text color={theme.border} width="100%">
                {"  Tab: focus"}
              </text>
              <text color={theme.border} width="100%">
                {"  Ctrl+P: palette"}
              </text>
              <text color={theme.border} width="100%">
                {"  Ctrl+C: exit"}
              </text>
            </box>
          </box>
        </Show>
      </box>

      {/* Input area with autocomplete */}
      <box
        width={dims().width}
        height={inputHeight}
        backgroundColor={theme.bgLighter}
        borderStyle="single"
        borderColor={theme.border}
        flexDirection="column"
      >
        <Show when={props.input().startsWith("/") && props.toolSuggestions && props.toolSuggestions.length > 0}>
          <Autocomplete
            input={props.input}
            suggestions={() => props.toolSuggestions || []}
            onSelect={(name) => props.setInput("/" + name + " ")}
          />
        </Show>
        <PromptInput
          value={props.input}
          onChange={props.setInput}
          onSubmit={props.onSend}
          placeholder="Type a message... (/help)"
          width={dims().width - 4}
        />
      </box>
    </box>
  )
}
