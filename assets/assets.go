// Package assets embeds the static files shipped with keepawake.
package assets

import (
	_ "embed"
)

// EyePNG is the tray icon shown while keepawake is active.
//
//go:embed icons/eye.png
var EyePNG []byte

// EyeClosedPNG is the tray icon shown while keepawake is inactive.
//
//go:embed icons/eye-closed.png
var EyeClosedPNG []byte
