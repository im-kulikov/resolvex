package bgp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"

	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
	"github.com/jwhited/corebgp"

	"github.com/im-kulikov/resolvex/internal/broadcast"
)

type Config struct {
	Clients    []string         `env:"CLIENTS"`
	Enabled    bool             `env:"ENABLED"    default:"true"`
	Network    string           `env:"NETWORK"    default:"tcp"`
	Address    string           `env:"ADDRESS"    default:":51179"`
	RouteID    string           `env:"ROUTER_ID"  default:"127.0.0.1"`
	LocalAs    uint32           `env:"LOCAL_AS"   default:"65001"`
	RemoteAs   uint32           `env:"REMOTE_AS"  default:"65001"`
	LocalPref  uint32           `env:"LOCAL_PREF" default:"100"`
	Attributes broadcast.Config `env:"ATTRIBUTES"`
}

const (
	serverName = "bgp-server"
	pluginName = "bgp-plugin"
)

func coreBGPLogger(log *logger.Logger) corebgp.Logger {
	out := logger.Named(log, "core-bgp")

	return func(args ...interface{}) {
		switch len(args) {
		case 0:
			return
		case 1:
			out.Info(fmt.Sprint(args[0]))
		default:
			out.Info(fmt.Sprint(args...))
		}
	}
}

// New creates a new BGP server.
func New(cfg Config, log *logger.Logger, rec broadcast.PeerManager) (service.Service, error) {
	var err error
	out := logger.Named(log, serverName)

	var rid netip.Addr
	if rid, err = netip.ParseAddr(cfg.RouteID); err != nil {
		return nil, err
	}

	corebgp.SetLogger(coreBGPLogger(log))

	var srv *corebgp.Server
	if srv, err = corebgp.NewServer(rid); err != nil {
		return nil, err
	}

	run := &plugin{
		Config: cfg,
		Logger: logger.Named(log, pluginName),

		rid: rid,
		srv: srv,
		rec: rec,
	}

	for _, client := range cfg.Clients {
		conf := corebgp.PeerConfig{
			RemoteAddress: netip.MustParseAddr(client),
			LocalAS:       cfg.LocalAs,
			RemoteAS:      cfg.RemoteAs,
		}

		out.Debug("prepare peer", logger.Any("peer", conf), logger.String("router_id", cfg.RouteID))
		if err = srv.AddPeer(conf, run, corebgp.WithLocalAddress(rid), corebgp.WithPassive()); err != nil {
			return nil, err
		}
	}

	return service.NewLauncher("bgp-server", func(ctx context.Context) error {
		var lis net.Listener
		if lis, err = new(net.ListenConfig).Listen(ctx, cfg.Network, cfg.Address); err != nil {
			return fmt.Errorf("bgp-server: could prepare listener: %w", err)
		}

		out.InfoContext(ctx, "listening", logger.String("address", cfg.Address))

		context.AfterFunc(ctx, srv.Close)

		if err = srv.Serve([]net.Listener{lis}); err != nil &&
			!errors.Is(err, corebgp.ErrServerClosed) {
			return fmt.Errorf("bgp server: could not start server: %w", err)
		}

		return nil
	}, func(ctx context.Context) { out.InfoContext(ctx, "bgp-server: shutdown gracefully done") }), nil
}
