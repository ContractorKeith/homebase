// Package web holds the dashboard's static files, embedded so the hub is one binary.
package web

import "embed"

//go:embed index.html app.css js vendor
var Files embed.FS
