package bgp

import (
	"context"
	"net/netip"
	"time"

	"github.com/im-kulikov/go-bones/logger"
	"github.com/jwhited/corebgp"
	"github.com/osrg/gobgp/v3/pkg/packet/bgp"

	"github.com/im-kulikov/resolvex/internal/broadcast"
)

type plugin struct {
	*logger.Logger

	ref uint32
	rec broadcast.PeerManager
}

func newPlugin(log *logger.Logger, ref uint32, rec broadcast.PeerManager) corebgp.Plugin {
	return &plugin{Logger: log, ref: ref, rec: rec}
}

func (p *plugin) GetCapabilities(peer corebgp.PeerConfig) []corebgp.Capability {
	p.Info("peer get capabilities", logger.Any("peer", peer))
	// caps := make([]corebgp.Capability, 0, 1)

	return nil
}

func (p *plugin) OnOpenMessage(
	peer corebgp.PeerConfig,
	_ netip.Addr,
	_ []corebgp.Capability,
) *corebgp.Notification {
	p.Info("peer open message", logger.Any("peer", peer))

	return nil
}

func (p *plugin) newWriter(
	peer string,
	writer corebgp.UpdateMessageWriter,
) broadcast.PeerWriter {
	return func(ctx context.Context, msg broadcast.UpdateMessage) error {
		removes := make([]*bgp.IPAddrPrefix, 0, len(msg.ToRemove))
		for _, address := range msg.ToRemove {
			removes = append(removes, bgp.NewIPAddrPrefix(32, address))
		}

		attributes := make([]bgp.PathAttributeInterface, 0, 4)
		attributes = append(attributes,
			bgp.NewPathAttributeOrigin(bgp.BGP_ORIGIN_ATTR_TYPE_IGP),
			bgp.NewPathAttributeAsPath([]bgp.AsPathParamInterface{}),
			bgp.NewPathAttributeNextHop("127.0.0.1"),
			bgp.NewPathAttributeLocalPref(p.ref))

		updates := make([]*bgp.IPAddrPrefix, 0, len(msg.ToUpdate))
		for _, address := range msg.ToUpdate {
			updates = append(updates, bgp.NewIPAddrPrefix(32, address))
		}

		out := bgp.NewBGPUpdateMessage(removes, attributes, updates)

		if buf, err := out.Body.Serialize(); err != nil {
			p.ErrorContext(
				ctx,
				"could not serialize update",
				logger.String("peer", peer),
				logger.Err(err),
			)

			return corebgp.UpdateNotificationFromErr(err)
		} else if err = writer.WriteUpdate(buf); err != nil {
			p.ErrorContext(ctx, "could not write update", logger.String("peer", peer), logger.Err(err))

			return corebgp.UpdateNotificationFromErr(err)
		}

		p.InfoContext(ctx, "update sent", logger.String("peer", peer))

		// send End-of-Rib
		if err := writer.WriteUpdate([]byte{0, 0, 0, 0}); err != nil {
			p.ErrorContext(
				ctx,
				"could not write end-of-rib",
				logger.String("peer", peer),
				logger.Err(err),
			)

			return corebgp.UpdateNotificationFromErr(err)
		}

		return nil
	}
}

func (p *plugin) OnEstablished(
	peer corebgp.PeerConfig,
	writer corebgp.UpdateMessageWriter,
) corebgp.UpdateMessageHandler {
	remote := peer.RemoteAddress.String()

	p.Info("peer established",
		logger.Any("peer", peer),
		logger.String("remote", remote))

	p.rec.AddPeer(remote, p.newWriter(remote, writer))

	time.Sleep(time.Second) // wait before send initial update

	// send End-of-Rib
	if err := writer.WriteUpdate([]byte{0, 0, 0, 0}); err != nil {
		return func(corebgp.PeerConfig, []byte) *corebgp.Notification {
			return corebgp.UpdateNotificationFromErr(err)
		}
	}

	return nil // ignore client updates
}

func (p *plugin) OnClose(peer corebgp.PeerConfig) {
	p.Info("peer closed", logger.Any("peer", peer))

	p.rec.DelPeer(peer.RemoteAddress.String())
}
