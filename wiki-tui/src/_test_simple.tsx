import solidPlugin from "@opentui/solid/bun-plugin"
try { Bun.plugin(solidPlugin) } catch {}
globalThis.React = { createElement: (t: any, p: any, ...c: any[]) => ({ type: t, props: p, children: c.flat() }), createContext: () => ({}), version: "19.0.0", Fragment: null, jsxDEV: (t: any, p: any) => ({ type: t, props: p }), jsxs: (t: any, p: any) => ({ type: t, props: p }), jsx: (t: any, p: any) => ({ type: t, props: p }) }

import { createCliRenderer } from "@opentui/core"
import { render } from "@opentui/solid"

const _d = (m: string) => { try { require("fs").appendFileSync("/tmp/td.log", m + "\n") } catch {} }

async function main() {
  try {
    _d("step1: createRenderer")
    const renderer = await createCliRenderer({ targetFps: 30, exitOnCtrlC: false, useKittyKeyboard: {}, useMouse: true })
    _d("step2: renderer created, dims=" + renderer.width + "x" + renderer.height)

    _d("step3: rendering")
    await render(() => {
      _d("render callback invoked")
      return (
        <box width={80} height={24} backgroundColor="#1a1a2e">
          <text bold color="#4a6fa5" width={80} height={1}>mini-wiki TEST</text>
        </box>
      )
    }, renderer)
    _d("step4: render completed")

    _d("step5: waiting...")
    await new Promise(r => setTimeout(r, 3000))
    _d("step6: done")
  } catch(e: any) {
    _d("ERROR: " + (e?.message || e))
    _d("STACK: " + (e?.stack || ""))
  }
}
main().catch(e => { _d("FATAL: " + e); process.exit(1) })
