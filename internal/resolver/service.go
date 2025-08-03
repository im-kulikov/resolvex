package resolver

import (
	"context"
	"time"

	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"

	"github.com/im-kulikov/resolvex/internal/storage"
)

type dnsResult struct {
	New storage.Item
	TTL uint32
}

const (
	serviceName    = "resolver"
	defaultTimeout = time.Second * 15
)

func New(cfg Config, log *logger.Logger, store storage.DNS) (service.Service, error) {
	out := logger.Named(log, serviceName)

	return service.NewLauncher(serviceName, func(top context.Context) error {
		tick := time.NewTimer(time.Microsecond)
		defer tick.Stop()

		for {
			select {
			case <-top.Done():
				out.InfoContext(top, "try gracefully shutdown")

				return nil
			case <-tick.C:
				now := time.Now()

				ctx, cancel := context.WithTimeout(top, defaultTimeout)

				out.DebugContext(top, "start resolving")

				cnt, err := cfg.resolveDomains(ctx, out, store)
				if err != nil {
					out.ErrorContext(top, "could not resolve", logger.Err(err))
				} else {
					out.InfoContext(top, "resolve done",
						logger.Int("domains", int(cnt)),
						logger.Any("spent", time.Since(now)))
				}
				cancel()
				tick.Reset(cfg.Timeout)
			}
		}
	}, func(ctx context.Context) { out.InfoContext(ctx, "gracefully shutdown") }), nil
}
