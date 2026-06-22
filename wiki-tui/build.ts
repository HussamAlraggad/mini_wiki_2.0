#!/usr/bin/env bun
/**
 * Build wiki-tui with SolidJS JSX plugin.
 * 
 * This two-step process is needed because:
 * 1. @opentui/solid/bun-plugin handles SolidJS JSX transform
 * 2. bun build --compile produces the native binary
 * 
 * Usage: bun run build.ts
 */
import solidPlugin from "@opentui/solid/bun-plugin"

// Step 1: Bundle with SolidJS plugin (JSX transform)
console.log("Bundling with SolidJS plugin...")
const result = await Bun.build({
  entrypoints: ["./src/index.tsx"],
  outdir: "./dist",
  target: "bun",
  plugins: [solidPlugin],
  minify: false,
  sourcemap: "none",
  // Externalize all platform-specific native addons — resolved at runtime
  external: [
    "yoga-layout",
    "@opentui/core-darwin-x64",
    "@opentui/core-darwin-arm64",
    "@opentui/core-linux-x64",
    "@opentui/core-linux-arm64",
    "@opentui/core-linux-x64-musl",
    "@opentui/core-linux-arm64-musl",
    "@opentui/core-win32-x64",
    "@opentui/core-win32-arm64",
  ],
})

if (!result.success) {
  console.error("Bundle failed:", result.logs)
  process.exit(1)
}

// Step 2: Compile the bundle to a native binary
// This includes all bundled dependencies
console.log("Compiling native binary...")
const compile = Bun.spawnSync([
  "bun", "build", "--compile", "--target=bun",
  "--outfile=dist/wiki-tui",
  "dist/index.js",
])

if (compile.exitCode !== 0) {
  console.error("Compile failed:", compile.stderr.toString())
  process.exit(compile.exitCode)
}

console.log("Done: dist/wiki-tui")
