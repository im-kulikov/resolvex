package broadcast

// PeerManager defines an interface for managing peers, allowing the addition and removal of peers by their identifier.
// Allow managing peers in a network communication system.
// It allows adding and deleting peers by their identifiers.
type PeerManager interface {
	DelPeer(string)
	AddPeer(string, PeerWriter)
}

type updatePeer struct {
	Peer   string
	Action actionType
	writer PeerWriter
}

// DelPeer removes a peer identified by the provided string from the server if it is not closed.
func (s *server) DelPeer(peer string) {
	if s.closed.Load() {
		return
	}

	s.action <- updatePeer{Peer: peer, Action: remPeer}
}

// AddPeer adds a new peer to the server's action channel with the specified writer if the server is not closed.
func (s *server) AddPeer(peer string, writer PeerWriter) {
	if s.closed.Load() {
		return
	}

	s.action <- updatePeer{Peer: peer, Action: addPeer, writer: writer}
}
