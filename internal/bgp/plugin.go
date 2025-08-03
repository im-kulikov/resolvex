package bgp

import (
	"context"
	"encoding/binary"
	"net"
	"net/netip"
	"time"

	"github.com/im-kulikov/go-bones/logger"
	bgp "github.com/jwhited/corebgp"

	"github.com/im-kulikov/resolvex/internal/broadcast"
)

type plugin struct {
	Config
	*logger.Logger

	rid netip.Addr
	srv *bgp.Server
	rec broadcast.PeerManager
}

func (p *plugin) GetCapabilities(peer bgp.PeerConfig) []bgp.Capability {
	p.Info("peer get capabilities", logger.Any("peer", peer))

	return []bgp.Capability{
		// Четырёхбайтная AS-нумерация (CAP_FOUR_OCTET_AS = 65)
		{
			Code:  bgp.CAP_FOUR_OCTET_AS,
			Value: binary.BigEndian.AppendUint32(nil, p.LocalAs),
		},

		// Route Refresh (CAP_ROUTE_REFRESH = 2)
		{Code: bgp.CAP_ROUTE_REFRESH},

		// Multiprotocol Extensions (CAP_MP_EXTENSIONS = 1), для IPv4 Unicast
		// Address Family Identifier: IPv4
		// Subsequent Address Family Identifier: Unicast
		bgp.NewMPExtensionsCapability(bgp.AFI_IPV4, bgp.SAFI_UNICAST),

		// Graceful Restart (CAP_GRACEFUL_RESTART = 64)
		// {
		// 	Code:  bgp.CAP_GRACEFUL_RESTART,
		// 	Value: []byte{0x00, 0x78, 0x00, 0x00}, // примерное значение (нужна конкретизация под задачу)
		// },
	}
}

func (p *plugin) OnOpenMessage(
	peer bgp.PeerConfig,
	_ netip.Addr,
	caps []bgp.Capability,
) *bgp.Notification {
	p.Info("peer open message",
		logger.String("peer", peer.RemoteAddress.String()), logger.Any("caps", caps))

	return nil
}

func (p *plugin) newWriter(
	peer string,
	writer bgp.UpdateMessageWriter,
) broadcast.PeerWriter {
	return func(ctx context.Context, msg broadcast.UpdateMessage) error {
		updates := make([]net.IPNet, 0, len(msg.ToUpdate))
		for _, address := range msg.ToUpdate {
			updates = append(updates, net.IPNet{
				IP:   net.ParseIP(address),
				Mask: net.IPv4Mask(255, 255, 255, 255),
			})
		}

		removes := make([]net.IPNet, 0, len(msg.ToRemove))
		for _, address := range msg.ToRemove {
			removes = append(removes, net.IPNet{
				IP:   net.ParseIP(address),
				Mask: net.IPv4Mask(255, 255, 255, 255),
			})
		}

		// We should provide one of:
		_ = OriginIGP
		_ = OriginEGP
		_ = OriginINCOMPLETE

		attributes := []Attribute{
			OriginEGP,
			&AttributeASPath{},
			&AttributeNextHop{IP: net.ParseIP("127.0.0.1")},
			&AttributeLocalPref{Pref: p.LocalPref},
		}

		if buf, err := buildUpdateMessage(updates, removes, attributes...); err != nil {
			p.ErrorContext(
				ctx,
				"could not serialize update",
				logger.String("peer", peer),
				logger.Err(err),
			)

			return err
		} else if err = writer.WriteUpdate(buf); err != nil {
			p.ErrorContext(ctx, "could not write update", logger.String("peer", peer), logger.Err(err))

			return err
		}

		p.InfoContext(ctx, "update sent", logger.String("peer", peer))

		// send End-of-Rib
		return writeEndOfRIB(p.Logger, peer, writer)
	}
}

func (p *plugin) OnEstablished(
	peer bgp.PeerConfig,
	writer bgp.UpdateMessageWriter,
) bgp.UpdateMessageHandler {
	remote := peer.RemoteAddress.String()

	p.Info("peer established",
		logger.Any("peer", peer),
		logger.String("remote", remote))

	p.rec.AddPeer(remote, p.newWriter(remote, writer))

	time.Sleep(time.Second) // wait before send initial update

	// send End-of-Rib
	if err := writeEndOfRIB(p.Logger, remote, writer); err != nil {
		return func(bgp.PeerConfig, []byte) *bgp.Notification {
			return bgp.UpdateNotificationFromErr(err)
		}
	}

	return nil // ignore client updates
}

func (p *plugin) OnClose(peer bgp.PeerConfig) {
	p.Info("peer closed", logger.Any("peer", peer))

	p.rec.DelPeer(peer.RemoteAddress.String())

	for _, client := range p.Clients {
		if client != peer.RemoteAddress.String() {
			p.Info(
				"drop peer",
				logger.String("peer", client),
				logger.String("router_id", p.RouteID),
			)

			continue
		}

		// используем, чтобы отпустить FSM
		go func() {
			conf := bgp.PeerConfig{
				RemoteAddress: netip.MustParseAddr(client),
				LocalAS:       p.LocalAs,
				RemoteAS:      p.RemoteAs,
			}

			if err := p.srv.DeletePeer(netip.MustParseAddr(client)); err != nil {
				p.Error("could not drop peer", logger.String("peer", client), logger.Err(err))
			}

			time.Sleep(time.Second)

			p.Info(
				"prepare peer",
				logger.String("peer", client),
				logger.String("router_id", p.RouteID),
			)
			if err := p.srv.AddPeer(conf, p); err != nil {
				p.Error("could not prepare peer", logger.String("peer", client), logger.Err(err))
			}
		}()
	}
}
