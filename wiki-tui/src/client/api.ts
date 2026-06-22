import { HttpClient } from "./http"

export type SourceInfo = {
  file: string
  score: number
  text?: string
  rerank_score?: number
}

type SSEHandler = {
  onMessage?: (data: any) => void
  onError?: (err: Error) => void
  onClose?: () => void
}

export class ApiClient {
  private http: HttpClient
  private baseUrl: string
  private abortController: AbortController | null = null

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl
    this.http = new HttpClient(baseUrl)
  }

  /** Check if the server is reachable (used for startup health check). */
  async ping(): Promise<boolean> {
    return this.http.ping(3000)
  }

  async getStatus(): Promise<any> {
    const res = await this.http.get("/api/status")
    if (!res.ok) throw new Error(`Status request failed: ${res.status}`)
    return JSON.parse(res.body)
  }

  async listModels(): Promise<any> {
    const res = await this.http.get("/api/models")
    if (!res.ok) throw new Error(`Models request failed: ${res.status}`)
    return JSON.parse(res.body)
  }

  async listTools(): Promise<any> {
    const res = await this.http.get("/api/tools")
    if (!res.ok) throw new Error(`Tools request failed: ${res.status}`)
    return JSON.parse(res.body)
  }

  async sendChat(text: string, context?: string): Promise<void> {
    await this.http.postAsync("/api/chat", { text, context })
  }

  async sendQuery(text: string, topK?: number, deep?: boolean): Promise<void> {
    await this.http.postAsync("/api/query", { text, top_k: topK || 5, deep })
  }

  async sendIngest(path: string, deep?: boolean): Promise<void> {
    await this.http.postAsync("/api/ingest", { path, deep })
  }

  async sendRank(topic: string): Promise<void> {
    await this.http.postAsync("/api/rank", { topic })
  }

  async setModel(model: string): Promise<void> {
    await this.http.postAsync("/api/model", { model })
  }

  /**
   * Connect to the SSE event stream using fetch with streaming.
   */
  connectSSE(handler: SSEHandler): { close: () => void } {
    this.abortController = new AbortController()

    const run = async () => {
      try {
        const response = await fetch(`${this.baseUrl}/api/events`, {
          signal: this.abortController!.signal,
        })

        if (!response.ok) {
          handler.onError?.(new Error(`SSE connection failed: ${response.status}`))
          return
        }

        const reader = response.body!.getReader()
        const decoder = new TextDecoder()
        let buffer = ""

        while (true) {
          const { done, value } = await reader.read()
          if (done) break

          buffer += decoder.decode(value, { stream: true })
          const lines = buffer.split("\n")
          buffer = lines.pop() || ""

          let currentData = ""

          for (const line of lines) {
            if (line.startsWith("data: ")) {
              currentData = line.slice(6)
              if (currentData) {
                try {
                  handler.onMessage?.(JSON.parse(currentData))
                } catch {
                  // Skip malformed JSON
                }
              }
              currentData = ""
            }
          }
        }
      } catch (err) {
        if (err instanceof Error && err.name !== "AbortError") {
          handler.onError?.(err)
        }
      }

      handler.onClose?.()
    }

    run()

    return {
      close: () => {
        this.abortController?.abort()
      },
    }
  }

  disconnect() {
    this.abortController?.abort()
  }
}
