/**
 * Preload script for bun build --compile.
 * Registers the SolidJS JSX transform plugin before the main entry runs.
 */
import solidPlugin from "@opentui/solid/bun-plugin"

// This plugin handles the JSX transform for SolidJS components
solidPlugin.setup?.()
