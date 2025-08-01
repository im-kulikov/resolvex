package main

import (
	"log/slog"
	"time"

	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/network/http"
	"github.com/im-kulikov/go-bones/service"
	"go.uber.org/zap"
	"go.uber.org/zap/exp/zapslog"
	"go.uber.org/zap/zapcore"

	"github.com/im-kulikov/resolvex/internal/api"
	"github.com/im-kulikov/resolvex/internal/bgp"
	"github.com/im-kulikov/resolvex/internal/broadcast"
	"github.com/im-kulikov/resolvex/internal/domain"
	"github.com/im-kulikov/resolvex/internal/resolver"
	"github.com/im-kulikov/resolvex/internal/storage"
)

type settings struct {
	config.Base

	API api.Config      `env:"API"`
	BGP bgp.Config      `env:"BGP"`
	DNS resolver.Config `env:"DNS"`
	CLI domain.Config   `env:"CLI"`

	Shutdown time.Duration `env:"SHUTDOWN" default:"5s"`
}

var version = "dev"

func handler(useDefault bool) slog.Handler {
	if useDefault {
		return nil
	}

	log, err := zap.NewDevelopment(
		zap.IncreaseLevel(zapcore.InfoLevel),
		zap.AddStacktrace(zapcore.DPanicLevel))
	if err != nil {
		panic(err)
	}

	return zapslog.NewHandler(log.Core(), zapslog.AddStacktraceAt(slog.Level(9)))
}

func main() {
	var cfg settings

	var err error
	if err = config.Load(&cfg); err != nil {
		logger.Error("could not load config", logger.Err(err))

		return
	}

	log := logger.Init(cfg.Logger, logger.WithHandler(handler(false)))

	// prepare broadcaster
	manager := broadcast.New(cfg.BGP.Attributes, log)

	var domains []string
	if domains, err = domain.Fetch(cfg.CLI); err != nil {
		logger.Error("could not fetch domain list", logger.Err(err))

		return
	}

	var store storage.Repository
	if store, err = storage.New(log, manager, domains); err != nil {
		logger.Error("could not create domain storage", logger.Err(err))

		return
	}

	var dnsService service.Service
	if dnsService, err = resolver.New(cfg.DNS, log, store); err != nil {
		logger.Error("could not create resolver service", logger.Err(err))

		return
	}

	var bgpService service.Service
	if bgpService, err = bgp.New(cfg.BGP, log, manager); err != nil {
		logger.Error("could not create bgp service", logger.Err(err))

		return
	}

	var apiService service.Service
	if apiService, err = api.New(cfg.API, log, store); err != nil {
		logger.Error("could not create api service", logger.Err(err))

		return
	}

	var opsService service.Service
	if opsService, err = http.NewOPSServer(cfg.OpsServer, log); err != nil {
		logger.Error("could not create ops service", logger.Err(err))

		return
	}

	log.Info("start service", logger.String("version", version))
	if err = service.Run(log, service.WithService(manager, dnsService, bgpService, apiService, opsService)); err != nil {
		logger.Error("could not create service runner", logger.Err(err))
	}
}
