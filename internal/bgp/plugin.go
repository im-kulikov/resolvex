package bgp

import (
	"context"
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

	return nil
}

func (p *plugin) OnOpenMessage(
	peer bgp.PeerConfig,
	_ netip.Addr,
	_ []bgp.Capability,
) *bgp.Notification {
	p.Info("peer open message", logger.Any("peer", peer))

	return nil
}

func (p *plugin) newWriter(
	peer string,
	writer bgp.UpdateMessageWriter,
) broadcast.PeerWriter {
	return func(ctx context.Context, msg broadcast.UpdateMessage) error {
		// removes := make([]*bgp.IPAddrPrefix, 0, len(msg.ToRemove))
		// for _, address := range msg.ToRemove {
		// 	removes = append(removes, bgp.NewIPAddrPrefix(32, address))
		// }
		//
		// attributes := make([]bgp.PathAttributeInterface, 0, 4)
		// attributes = append(attributes,
		// 	bgp.NewPathAttributeOrigin(bgp.BGP_ORIGIN_ATTR_TYPE_IGP),
		// 	bgp.NewPathAttributeAsPath([]bgp.AsPathParamInterface{}),
		// 	bgp.NewPathAttributeNextHop("127.0.0.1"),
		// 	bgp.NewPathAttributeLocalPref(p.ref))
		//
		// updates := make([]*bgp.IPAddrPrefix, 0, len(msg.ToUpdate))
		// for _, address := range msg.ToUpdate {
		// 	updates = append(updates, bgp.NewIPAddrPrefix(32, address))
		// }
		//
		// out := bgp.NewBGPUpdateMessage(removes, attributes, updates)

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
			OriginIGP,
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
		// используем, чтобы отпустить FSM
		go func() {
			conf := bgp.PeerConfig{
				RemoteAddress: netip.MustParseAddr(client),
				LocalAS:       p.LocalAs,
				RemoteAS:      p.RemoteAs,
			}

			p.Debug(
				"try to drop peer",
				logger.String("peer", client),
				logger.String("router_id", p.RouteID),
			)
			if err := p.srv.DeletePeer(netip.MustParseAddr(client)); err != nil {
				p.Error("could not drop peer", logger.String("peer", client), logger.Err(err))
			}

			time.Sleep(time.Second)

			p.Debug(
				"prepare peer",
				logger.String("peer", client),
				logger.String("router_id", p.RouteID),
			)
			if err := p.srv.AddPeer(conf, p, bgp.WithLocalAddress(p.rid), bgp.WithPassive()); err != nil {
				p.Error("could not prepare peer", logger.String("peer", client), logger.Err(err))
			}
		}()
	}
}
