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

const serviceName = "resolver"

func New(cfg Config, log *logger.Logger, store storage.DNS) (service.Service, error) {
	out := logger.Named(log, serviceName)

	return service.NewLauncher(serviceName, func(ctx context.Context) error {
		tick := time.NewTimer(time.Microsecond)
		defer tick.Stop()

		for {
			select {
			case <-ctx.Done():
				out.InfoContext(ctx, "try gracefully shutdown")

				return nil
			case <-tick.C:
				now := time.Now()
				out.InfoContext(ctx, "start resolving")

				cnt, err := cfg.resolveDomains(ctx, out, store)
				if err != nil {
					out.ErrorContext(ctx, "could not resolve", logger.Err(err))
				}

				out.InfoContext(ctx, "stop resolving",
					logger.Int("count", int(cnt)),
					logger.Any("spent", time.Since(now)))

				tick.Reset(cfg.Timeout)
			}
		}
	}, func(ctx context.Context) { out.InfoContext(ctx, "gracefully shutdown") }), nil
}
