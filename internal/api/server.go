package api

import (
	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/network/http"
	"github.com/im-kulikov/go-bones/service"

	"github.com/im-kulikov/resolvex/internal/storage"
)

type Config struct {
	config.BaseHTTP

	Address string `env:"ADDRESS" default:":8080"`
}

type server struct {
	storage.API
	*logger.Logger
}

func (c Config) Addr() string { return c.Address }

func New(cfg config.HTTPConfig, log *logger.Logger, rec storage.API) (service.Service, error) {
	srv := &server{API: rec, Logger: log}

	return http.NewServer(cfg, log,
		http.ServiceName("admin"),
		http.ServerOptions(srv.attach))
}
