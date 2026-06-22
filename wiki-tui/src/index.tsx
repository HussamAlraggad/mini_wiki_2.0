/// <reference types="bun-types" />

// The OpenTUI SolidJS reconciler generates React-style JSX internally.
// We define the React global so the compiled binary can resolve it.
// @ts-ignore
globalThis.React = {
  createElement: (type: any, props: any, ...children: any[]) => ({ type, props, children: children.flat() }),
  createContext: (v: any) => ({ _val: v, _providers: new Set() }),
  createRef: () => ({ current: null }),
  version: "19.0.0",
  Fragment: Symbol("Fragment"),
  // @ts-ignore
  jsxDEV: (type: any, props: any) => ({ type, props }),
  // @ts-ignore
  jsxs: (type: any, props: any) => ({ type, props }),
  // @ts-ignore
  jsx: (type: any, props: any) => ({ type, props }),
}

// Polyfill requestAnimationFrame for bun-compiled binaries.
// OpenTUI uses rAF for its render loop; without it, nothing draws.
if (typeof globalThis.requestAnimationFrame === "undefined") {
  globalThis.requestAnimationFrame = (cb: (ts: number) => void): number => {
    return +setTimeout(() => cb(Date.now()), 16) // ~60fps
  }
}
if (typeof globalThis.cancelAnimationFrame === "undefined") {
  globalThis.cancelAnimationFrame = (id: number) => clearTimeout(id)
}

// Register the SolidJS JSX transform plugin for compiled binary support.
import solidPlugin from "@opentui/solid/bun-plugin"
try { Bun.plugin(solidPlugin) } catch {}

import { createCliRenderer } from "@opentui/core"
import { render } from "@opentui/solid"
import { App } from "./app"

// Use WIKI_PORT env var first (set by Go server), then CLI arg, then default
const portStr = process.env.WIKI_PORT || Bun.argv[1] || "43567"
const port = parseInt(portStr, 10)
if (isNaN(port) || port <= 0 || port > 65535) {
  console.error(`Invalid port: ${portStr}. Expected a number between 1-65535.`)
  process.exit(1)
}
const baseUrl = `http://127.0.0.1:${port}`

/** Simple TCP connection check that works in bun-compiled binaries. */
async function tcpPing(host: string, port: number, timeoutMs = 1000): Promise<boolean> {
  return new Promise<boolean>((resolve) => {
    const timer = setTimeout(() => resolve(false), timeoutMs)
    try {
      Bun.connect({
        hostname: host,
        port: port,
        socket: {
          open(socket) {
            clearTimeout(timer)
            socket.end()
            resolve(true)
          },
          data(socket, _data) {},
          close(_socket) {},
          error(_socket, _err) {
            clearTimeout(timer)
            resolve(false)
          },
          drain(_socket) {},
        },
      })
    } catch {
      clearTimeout(timer)
      resolve(false)
    }
  })
}

async function main() {
  let connected = false

  for (let i = 0; i < 20; i++) {
    connected = await tcpPing("127.0.0.1", port, 500)
    if (connected) break
    await new Promise((r) => setTimeout(r, 250))
  }

  if (!connected) {
    console.error("")
    console.error("Cannot reach mini-wiki server at " + baseUrl)
    console.error("Make sure 'wiki' is running")
    process.exit(1)
  }

  // Enter raw terminal mode and render the UI
  // Enter raw terminal mode and render the UI
  const renderer = await createCliRenderer({
    targetFps: 60,
    exitOnCtrlC: false,
    useKittyKeyboard: {},
    useMouse: true,
  })

  // Log debug info to file since terminal is in raw mode
  const log = (m: string) => { try { Bun.write("/tmp/wiki-debug.log", m + "\n") } catch {} }

  try {
    log("Calling render()...")
    await render(
      () => {
        log("Render callback called")
        return <App baseUrl={baseUrl} renderer={renderer} />
      },
      renderer,
    )
    log("Render completed")
  } catch (e: any) {
    log("Render error: " + (e?.message || e))
    throw e
  }
}

main().catch((err) => {
  const msg = err?.message || String(err)
  try { Bun.write("/tmp/wiki-debug.log", "Fatal: " + msg + "\n") } catch {}
  console.error("")
  console.error("Fatal error:", err)
  process.exit(1)
})
