import solidPlugin from "@opentui/solid/bun-plugin"
try { Bun.plugin(solidPlugin) } catch {}
globalThis.React = { createElement: (t: any, p: any, ...c: any[]) => ({ type: t, props: p, children: c.flat() }), createContext: () => ({}), version: "19.0.0", Fragment: null, jsxDEV: (t: any, p: any) => ({ type: t, props: p }), jsxs: (t: any, p: any) => ({ type: t, props: p }), jsx: (t: any, p: any) => ({ type: t, props: p }) }

import { createCliRenderer } from "@opentui/core"
import { render } from "@opentui/solid"

async function main() {
  const renderer = await createCliRenderer({ targetFps: 30, exitOnCtrlC: false, useKittyKeyboard: {}, useMouse: true })

  await render(() => {
    return (
      <box width="100%" height="100%" backgroundColor="#1a1a2e" flexDirection="column" alignItems="center" justifyContent="center">
        <box backgroundColor="#4a6fa5" paddingX={4} paddingY={1}>
          <text bold color="#ffffff">mini-wiki</text>
        </box>
        <box height={2} />
        <text color="#e0e0e0">If you see this, OpenTUI works!</text>
      </box>
    )
  }, renderer)

  // Keep alive using renderer's event loop instead of setTimeout
  await new Promise(() => {})  // never resolves - keep alive forever
}
main().catch(e => process.exit(1))
