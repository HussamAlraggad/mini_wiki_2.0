//go:build !tuibuild

package main

// tuiBinaryData is empty when the TUI is not embedded.
// Run `bash scripts/build-tui.sh` to build with embedded TUI.
var tuiBinaryData []byte
