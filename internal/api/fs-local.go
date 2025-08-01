//go:build local

package api

import (
	"net/http"
)

var content = http.Dir("internal/api/frontend")
