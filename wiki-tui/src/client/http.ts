/**
 * Minimal HTTP client using Bun's raw TCP socket.
 * 
 * Bun compiled binaries have issues with fetch() to localhost.
 * This uses Bun.connect() to make raw HTTP requests instead.
 */

export class HttpClient {
  private host: string
  private port: number

  constructor(baseUrl: string) {
    const url = new URL(baseUrl)
    this.host = url.hostname
    this.port = parseInt(url.port, 10) || 80
  }

  /** Quick TCP ping — checks if the server port is open. */
  async ping(timeoutMs = 1000): Promise<boolean> {
    return new Promise((resolve) => {
      const timer = setTimeout(() => {
        try { socket.end() } catch {}
        resolve(false)
      }, timeoutMs)

      const socket = Bun.connect({
        hostname: this.host,
        port: this.port,
        socket: {
          open(socket) {
            clearTimeout(timer)
            socket.end()
            resolve(true)
          },
          data(socket, data) {},  // required by Bun
          error(socket, err) {
            clearTimeout(timer)
            resolve(false)
          },
        },
      })
    })
  }

  /** Full HTTP GET request. */
  async get(path: string): Promise<{ ok: boolean; status: number; body: string }> {
    return this.request("GET", path)
  }

  /** Full HTTP POST request with JSON body. */
  async post(path: string, body: unknown): Promise<{ ok: boolean; status: number; body: string }> {
    return this.request("POST", path, JSON.stringify(body))
  }

  /** Full HTTP POST without waiting for response (fire and forget). */
  async postAsync(path: string, body: unknown): Promise<void> {
    const payload = JSON.stringify(body)
    const req = `POST ${path} HTTP/1.1\r\nHost: ${this.host}\r\nContent-Type: application/json\r\nContent-Length: ${payload.length}\r\nConnection: close\r\n\r\n${payload}`

    Bun.connect({
      hostname: this.host,
      port: this.port,
      socket: {
        open(socket) { socket.write(req) },
        data(socket, data) {},
        close(socket) {},
        error(socket, err) {},
        drain(socket) {},
      },
    })
  }

  private async request(method: string, path: string, body?: string): Promise<{ ok: boolean; status: number; body: string }> {
    return new Promise((resolve, reject) => {
      let responseData = ""
      let statusCode = 0
      let headersDone = false
      let resolved = false

      const finish = (err?: Error) => {
        if (resolved) return
        resolved = true
        if (err) {
          reject(err)
        } else {
          resolve({
            ok: statusCode >= 200 && statusCode < 300,
            status: statusCode,
            body: responseData,
          })
        }
      }

      const bHost = this.host
      const bPort = this.port

      const socket = Bun.connect({
        hostname: bHost,
        port: bPort,
        socket: {
          open(socket) {
            let req = `${method} ${path} HTTP/1.1\r\nHost: ${bHost}\r\nConnection: close\r\n`
            if (body) {
              req += `Content-Type: application/json\r\nContent-Length: ${body.length}\r\n`
            }
            req += "\r\n"
            if (body) req += body
            socket.write(req)
          },
          data(socket, data) {
            const text = new TextDecoder().decode(data)
            if (!headersDone) {
              const headerEnd = text.indexOf("\r\n\r\n")
              if (headerEnd !== -1) {
                headersDone = true
                const statusLine = text.substring(0, text.indexOf("\r\n"))
                const m = statusLine.match(/HTTP\/\d\.\d (\d+)/)
                statusCode = m ? parseInt(m[1], 10) : 0
                responseData = text.substring(headerEnd + 4)
              }
            } else {
              responseData += text
            }
          },
          close(socket) { finish() },
          error(socket, err) { finish(new Error(String(err))) },
          drain(socket) {},  // required by Bun
        },
      })
    })
  }
}
