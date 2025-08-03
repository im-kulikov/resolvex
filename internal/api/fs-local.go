//go:build local

package api

import (
	"net/http"

	"github.com/im-kulikov/go-bones/logger"
)

var content = http.Dir("internal/api/frontend")

func init() { logger.Info("use local storage") }
