//go:build tuibuild

package main

import (
	_ "embed"
)

//go:embed wiki-tui/dist/wiki-tui
var tuiBinaryData []byte
