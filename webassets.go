package main

import "mime"

// Fonts bundled with the UI (web/fonts) need a type that every OS knows.
func init() {
	_ = mime.AddExtensionType(".woff2", "font/woff2")
	_ = mime.AddExtensionType(".svg", "image/svg+xml")
}
