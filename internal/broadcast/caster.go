package broadcast

// Broadcaster defines an interface for broadcasting update messages to peers or systems.
type Broadcaster interface {
	Broadcast(msg UpdateMessage)
}

// UpdateMessage represents a message containing updates and removals.
// ToUpdate contains a list of items to be added or updated.
// ToRemove contains a list of items to be removed.
type UpdateMessage struct {
	Cause    UpdateCause
	ToUpdate []string
	ToRemove []string
}

type UpdateCause int

const (
	CauseRemoval UpdateCause = iota
	CauseAPIDelete
	CauseAPIUpdate
	CauseDNSPublish
)

func (cause UpdateCause) String() string {
	switch cause {
	case CauseRemoval:
		return "removal"
	case CauseAPIDelete:
		return "api-delete"
	case CauseAPIUpdate:
		return "api-update"
	case CauseDNSPublish:
		return "resolver-publish"
	default:
		return "unknown"
	}
}

func (s *server) Broadcast(msg UpdateMessage) {
	if s.closed.Load() {
		return
	}

	s.output <- msg
}
