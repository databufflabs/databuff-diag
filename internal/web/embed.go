// Package web holds self-contained static UI assets embedded at build time.
package web

import "embed"

//go:embed index.html preview.html app.css app.js code-view.js preview.js favicon.svg favicon.png
var FS embed.FS
