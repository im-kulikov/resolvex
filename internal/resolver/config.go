package resolver

import (
	"context"
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/im-kulikov/go-bones"
	"github.com/im-kulikov/go-bones/logger"
	"github.com/miekg/dns"
	"golang.org/x/sync/errgroup"

	"github.com/im-kulikov/resolvex/internal/storage"
)

type Config struct {
	Servers []string      `env:"SERVERS"`
	Timeout time.Duration `env:"TIMEOUT" default:"15s"`
}

const defaultDomain = "google"

// nolint:gochecknoglobals
var defaultDNS atomic.Pointer[string]

func (c *Config) Validate(ctx context.Context, log *logger.Logger) error {
	if len(c.Servers) == 0 {
		return fmt.Errorf("provide list of dns servers")
	}

	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(defaultDomain), dns.TypeA)
	msg.SetEdns0(4096, true)

	for _, server := range c.Servers {
		if err := bones.ExtractError(dns.ExchangeContext(ctx, msg, server)); err != nil {
			return fmt.Errorf("dns server(%q) failed: %w", server, err)
		}

		log.DebugContext(ctx, "server pass", logger.String("server", server))
	}

	index := rand.IntN(len(c.Servers)) // nolint:gosec
	defaultDNS.Store(&c.Servers[index])

	return nil
}

type resolveParams struct {
	*logger.Logger

	cnt *atomic.Int32
	out chan dnsResult
}

type request struct {
	domain  string
	server  string
	message *dns.Msg
}

func (rp *resolveParams) resolveDomain(ctx context.Context, req request) func() error {
	return func() error {
		defer rp.cnt.Add(-1)

		rp.DebugContext(ctx, "try to resolve",
			logger.String("server", req.server),
			logger.String("domain", req.domain))

		res, err := dns.ExchangeContext(ctx, req.message, req.server)
		if err != nil {
			if strings.Contains(err.Error(), "i/o timeout") {
				return nil
			}

			rp.ErrorContext(ctx, "could not resolve domain",
				logger.String("server", req.server),
				logger.String("domain", req.domain),
				logger.Err(err))

			return nil
		}

		val := dnsResult{New: storage.Item{Domain: req.domain}}
		for _, ra := range res.Answer {
			if ro, ok := ra.(*dns.A); ok {
				val.TTL = ro.Hdr.Ttl
				val.New.Record = append(val.New.Record, ro.A.String())
			}
		}

		rp.DebugContext(ctx, "try to send answer",
			logger.String("server", req.server),
			logger.String("domain", req.domain))

		if err = ctx.Err(); err != nil {
			return err
		}

		if err = ctx.Err(); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case rp.out <- val:
			return nil
		}
	}
}

func (c *Config) resolveDomains(
	top context.Context,
	log *logger.Logger,
	store storage.DNS,
) (int32, error) {
	run, ctx := errgroup.WithContext(top)

	inc := new(atomic.Int32)
	cli := resolveParams{cnt: new(atomic.Int32), out: make(chan dnsResult, 1000), Logger: log}
	for _, domain := range store.ExpiredDomains() {
		inc.Add(1)

		log.DebugContext(ctx, "run resolve for domain",
			logger.String("domain", domain))

		msg := new(dns.Msg)
		msg.SetQuestion(dns.Fqdn(domain), dns.TypeA)
		msg.SetEdns0(4096, true)

		for _, server := range c.Servers {
			log.DebugContext(ctx, "run resolver for server",
				logger.String("server", server),
				logger.String("domain", domain))

			cli.cnt.Add(1)
			run.Go(cli.resolveDomain(ctx, request{domain: domain, server: server, message: msg}))
		}
	}

	if inc.Load() == 0 {
		log.InfoContext(ctx, "nothing to do")

		return 0, nil
	}

	now := time.Now()
	lst := make(map[string]storage.PublishItem)
	run.Go(func() error {
		tick := time.NewTimer(time.Second)

	loop:
		for {
			select {
			case <-ctx.Done():
				log.DebugContext(ctx, "context done")
				tick.Stop()
				close(cli.out)
				break loop
			case <-tick.C:
				log.DebugContext(ctx, "wait for results")
				tick.Reset(time.Second)

				if cli.cnt.Load() <= 0 {
					close(cli.out)
					break loop
				}
			case res, ok := <-cli.out:
				if !ok {
					continue loop
				}

				newExpires := time.Now().Add(time.Hour)
				if _, ok = lst[res.New.Domain]; !ok {
					lst[res.New.Domain] = storage.PublishItem{
						Domain: res.New.Domain,
						Record: make(map[string]time.Time),
						Expire: now.Add(time.Second * time.Duration(res.TTL)),
					}
				}

				log.DebugContext(ctx, "received message", logger.Any("message", res))
				for _, address := range res.New.Record {
					var oldExpires time.Time
					if oldExpires, ok = lst[res.New.Domain].Record[address]; !ok {
						lst[res.New.Domain].Record[address] = newExpires
					} else if oldExpires.Before(newExpires) {
						lst[res.New.Domain].Record[address] = newExpires
					}
				}
			}
		}

		return nil
	})

	if err := run.Wait(); err != nil {
		return 0, fmt.Errorf("something went wrong: %w", err)
	}

	store.Publish(slices.Collect(maps.Values(lst)))

	return inc.Load(), nil
}
