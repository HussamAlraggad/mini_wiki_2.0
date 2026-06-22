/** @jsxImportSource solid-js */
import { createSignal, createEffect, onCleanup, Show } from "solid-js"
import { useTerminalDimensions } from "@opentui/solid"
import type { CliRenderer } from "@opentui/core"
import { ApiClient, type SourceInfo } from "./client/api"
import { ChatPanel } from "./routes/session"
import { StatusBar } from "./components/status"
import { WelcomeScreen } from "./routes/home"
import { theme } from "./styles/theme"

export type AppProps = {
  baseUrl: string
  renderer: CliRenderer
}

export type Message = {
  id: string
  role: "user" | "assistant" | "system"
  content: string
  timestamp: number
}

export function App(props: AppProps) {
  const dims = useTerminalDimensions()
  const api = new ApiClient(props.baseUrl)

  const [messages, setMessages] = createSignal<Message[]>([])
  const [status, setStatus] = createSignal<"idle" | "thinking" | "streaming" | "error">("idle")
  const [statusText, setStatusText] = createSignal("Ready")
  const [model, setModel] = createSignal("")
  const [ragReady, setRagReady] = createSignal(false)
  const [input, setInput] = createSignal("")
  const [streamingContent, setStreamingContent] = createSignal("")
  const [currentSources, setCurrentSources] = createSignal<SourceInfo[]>([])
  const [datasetInfo, setDatasetInfo] = createSignal("")
  const [availableTools, setAvailableTools] = createSignal<any[]>([])

  let sseConnection: { close: () => void } | null = null

  // Load initial status
  createEffect(() => {
    loadStatus()
    loadTools()
  })

  onCleanup(() => {
    sseConnection?.close()
  })

  async function loadStatus() {
    try {
      const res = await api.getStatus()
      setModel(res.model || "")
      setRagReady(res.rag_running || false)
      if (res.rag_status?.sources?.length > 0) {
        setDatasetInfo(`${res.rag_status.total_chunks || 0} chunks indexed`)
      }
    } catch (err) {
      console.error("Failed to load status:", err)
    }
  }

  async function loadTools() {
    try {
      const res = await api.listTools()
      setAvailableTools(res.tools || [])
    } catch {
      // non-critical
    }
  }

  function handleSend(text: string) {
    if (!text.trim()) return

    // Handle commands
    if (text.startsWith("/")) {
      handleCommand(text)
      return
    }

    // Regular message
    addUserMsg(text)
    setStatus("thinking")
    setStatusText("Thinking...")
    setStreamingContent("")
    setCurrentSources([])

    sseConnection?.close()

    sseConnection = api.connectSSE({
      onMessage: (data) => handleSSEEvent(data),
      onError: (err) => {
        setStatus("error")
        setStatusText(`Connection error: ${err.message}`)
      },
    })

    api.sendChat(text).catch((err) => {
      setStatus("error")
      setStatusText(`Error: ${err.message}`)
    })
  }

  async function handleCommand(text: string) {
    const parts = text.split(/\s+/)
    const cmd = parts[0].toLowerCase()
    const args = parts.slice(1).join(" ")

    switch (cmd) {
      case "/help":
        const tools = availableTools()
        let helpText = "**Commands**\n\n"
        for (const t of tools) {
          const args = t.input_schema?.required?.join(" ") || ""
          helpText += `  \`/${t.name}${args ? " <" + args + ">" : ""}\`  — ${t.description}\n`
        }
        helpText += "\n**Tips**\n"
        helpText += "  - Type naturally for AI chat with RAG context\n"
        helpText += "  - RAG sources appear in the sidebar on wide screens\n"
        helpText += "  - Use `/help` to see this list anytime"
        addAssistantMsg(helpText)
        break

      case "/models":
        try {
          const res = await api.listModels()
          const list = res.models?.length
            ? res.models.map((m: string) => `  - ${m}`).join("\n")
            : "  (none loaded — start Ollama first)"
          addAssistantMsg(`**Available Models**\n\nActive: ${res.active || "(none)"}\n\n${list}`)
        } catch (err: any) {
          addSystemMsg(`Error: ${err.message}`)
        }
        break

      case "/model":
        if (!args) {
          addSystemMsg(`Current model: ${model()}`)
          return
        }
        try {
          await api.setModel(args)
          setModel(args)
          addSystemMsg(`Switched to model: ${args}`)
        } catch (err: any) {
          addSystemMsg(`Error: ${err.message}`)
        }
        break

      case "/ingest":
        if (!args) {
          addSystemMsg("Usage: /ingest <filepath>")
          return
        }
        addSystemMsg(`Ingesting ${args}...`)
        setStatusText("Ingesting...")
        sseConnection?.close()
        sseConnection = api.connectSSE({
          onMessage: (data) => handleSSEEvent(data),
          onError: (err) => addSystemMsg(`Error: ${err.message}`),
        })
        await api.sendIngest(args)
        break

      case "/query":
        if (!args) {
          addSystemMsg("Usage: /query <question>")
          return
        }
        addUserMsg(args)
        setStatus("thinking")
        setStatusText("Searching knowledge base...")
        setStreamingContent("")
        setCurrentSources([])
        sseConnection?.close()
        sseConnection = api.connectSSE({
          onMessage: (data) => handleSSEEvent(data),
          onError: (err) => addSystemMsg(`Error: ${err.message}`),
        })
        await api.sendQuery(args)
        break

      case "/rank":
        if (!args) {
          addSystemMsg("Usage: /rank <topic>")
          return
        }
        addSystemMsg(`Ranking dataset by: ${args}`)
        setStatusText("Ranking...")
        sseConnection?.close()
        sseConnection = api.connectSSE({
          onMessage: (data) => handleSSEEvent(data),
          onError: (err) => addSystemMsg(`Error: ${err.message}`),
        })
        await api.sendRank(args)
        break

      case "/clear":
        setMessages([])
        setStreamingContent("")
        setCurrentSources([])
        setStatusText("Conversation cleared")
        break

      case "/exit":
        process.exit(0)
        break

      default:
        addSystemMsg(`Unknown command: ${cmd}. Type /help for available commands.`)
    }

    setStatus("idle")
    setStatusText("Ready")
  }

  function addUserMsg(content: string) {
    setMessages((prev) => [...prev, {
      id: Date.now().toString(),
      role: "user",
      content,
      timestamp: Date.now(),
    }])
  }

  function addAssistantMsg(content: string) {
    setMessages((prev) => [...prev, {
      id: Date.now().toString(),
      role: "assistant",
      content,
      timestamp: Date.now(),
    }])
  }

  function addSystemMsg(content: string) {
    setMessages((prev) => [...prev, {
      id: Date.now().toString(),
      role: "system",
      content,
      timestamp: Date.now(),
    }])
  }

  function handleSSEEvent(event: any) {
    switch (event.type) {
      case "progress":
        setStatusText(event.data.message || event.data)
        break

      case "token":
        setStreamingContent((prev) => prev + event.data.delta)
        setStatus("streaming")
        break

      case "chat_result":
        addAssistantMsg(event.data.answer || streamingContent())
        setStreamingContent("")
        setStatus("idle")
        setStatusText("Ready")
        break

      case "query_result":
        setCurrentSources(event.data.sources || [])
        addAssistantMsg(event.data.answer || "")
        setStreamingContent("")
        setStatus("idle")
        setStatusText("Ready")
        break

      case "error":
        addSystemMsg(`Error: ${event.data.message || event.data}`)
        setStreamingContent("")
        setStatus("idle")
        setStatusText("Error")
        break

      case "ingest_done":
        addSystemMsg(`Ingested ${event.data.path}: ${event.data.chunks} chunks`)
        setStatusText("Ready")
        // Refresh status to update chunk count
        loadStatus()
        break

      case "rank_result":
        addAssistantMsg(
          `**Ranking Results**\n\n` +
          `Rows kept: ${event.data.rows_kept} / ${event.data.total_rows}\n` +
          `${event.data.message || ""}`
        )
        setStatus("idle")
        setStatusText("Ranking complete")
        break

      case "connected":
        // Connection established
        break
    }
  }

  const width = () => dims().width
  const height = () => dims().height
  const isWide = () => width() >= 120
  const isNarrow = () => width() < 80
  const hasMessages = () => messages().length > 0 || streamingContent() !== ""

  return (
    <box width={width()} height={height()} flexDirection="column" backgroundColor={theme.bgDarker}>
      {/* Main content fills available space */}
      <box width="100%" flexGrow={1}>
        <Show
          when={hasMessages()}
          fallback={
            <WelcomeScreen
              onSend={handleSend}
              input={input}
              setInput={setInput}
            />
          }
        >
          <ChatPanel
            messages={messages()}
            streamingContent={streamingContent()}
            status={status()}
            onSend={handleSend}
            input={input}
            setInput={setInput}
            isWide={isWide()}
            isNarrow={isNarrow()}
            width={width()}
            height={height()}
            sources={currentSources()}
            datasetInfo={datasetInfo()}
            toolSuggestions={availableTools()}
          />
        </Show>
      </box>

      {/* Status bar */}
      <StatusBar
        model={model()}
        status={statusText()}
        ragReady={ragReady()}
        messageCount={messages().length}
        width={width}
      />
    </box>
  )
}
