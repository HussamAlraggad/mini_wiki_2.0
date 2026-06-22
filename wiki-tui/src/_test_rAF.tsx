// Check if requestAnimationFrame exists
console.log("typeof requestAnimationFrame:", typeof requestAnimationFrame)
console.log("typeof setTimeout:", typeof setTimeout)
console.log("typeof setInterval:", typeof setInterval)

import solidPlugin from "@opentui/solid/bun-plugin"
try { Bun.plugin(solidPlugin) } catch {}
globalThis.React = { createElement: (t: any, p: any, ...c: any[]) => ({ type: t, props: p, children: c.flat() }), createContext: () => ({}), version: "19.0.0", Fragment: null }

import { createCliRenderer } from "@opentui/core"
import { render } from "@opentui/solid"

async function main() {
  console.log("typeof requestAnimationFrame:", typeof requestAnimationFrame)
  
  const renderer = await createCliRenderer({ targetFps: 30, exitOnCtrlC: false, useKittyKeyboard: {}, useMouse: true })
  console.log("Renderer created, width:", renderer.width, "height:", renderer.height)

  await render(() => {
    console.log("Render callback")
    return (
      <box width="100%" height="100%" backgroundColor="#1a1a2e">
        <text bold color="#4a6fa5">mini-wiki</text>
      </box>
    )
  }, renderer)
  
  console.log("Render completed")
  
  // Draw a frame manually
  // renderer.requestRender()
  
  await new Promise(r => setTimeout(r, 2000))
  console.log("Done")
}
main().catch(e => console.error("Error:", e))
