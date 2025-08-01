//go:build !local

package api

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed frontend/*
var root embed.FS

var frontend, _ = fs.Sub(root, "frontend") // nolint:gochecknoglobals

var content = http.FS(frontend) // nolint:gochecknoglobals
