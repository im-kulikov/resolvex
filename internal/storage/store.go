package storage

import (
	"fmt"
	"sync"
	"time"

	"github.com/im-kulikov/go-bones"
	"github.com/im-kulikov/go-bones/logger"
	"github.com/maypok86/otter/v2"

	"github.com/im-kulikov/resolvex/internal/broadcast"
)

// Item represents a structure used for associating domains with IP addresses and their expiration times.
type Item struct {
	ext map[string]time.Time

	Domain string
	Record []string
	Expire time.Time
}

// Repository is a composite interface that combines the functionalities of BGP, API, and DNS interfaces.
type Repository interface {
	BGP
	API
	DNS
}

// ipStorage represents a thread-safe storage for managing a map of IP addresses and their reference counts.
type ipStorage struct {
	sync.RWMutex

	list map[string]int
}

// store represents a data structure for managing domains, associated IPs, and broadcasting updates with thread safety.
type store struct {
	*logger.Logger

	ipItems *ipStorage
	domains *otter.Cache[string, Item]

	manager broadcast.Broadcaster
}

// cleanDomainListener defines a function type handling deletion events for domain-related items.
type cleanDomainListener func(e otter.DeletionEvent[string, Item])

const (
	// serviceName defines the name of the service as "repository".
	serviceName = "repository"

	// ErrExist represents an error indicating that the entity already exists.
	ErrExist bones.Error = "exists"
	// ErrNotFound represents an error indicating that the entity was not found.
	ErrNotFound bones.Error = "not found"
)

// toRemoveDomains processes domain deletion events to update IP storage and broadcast removed IPs.
// It adjusts the reference count for each IP and removes those with zero references.
// Removed IPs are then broadcast via the specified broadcast.Broadcaster.
func toRemoveDomains(
	log *logger.Logger,
	store *ipStorage,
	manager broadcast.Broadcaster,
) cleanDomainListener {
	return func(event otter.DeletionEvent[string, Item]) {
		var removed []string
		log.Debug("record was replaced or removed",
			logger.String("domain", event.Key),
			logger.String("cause", event.Cause.String()))

		if event.Cause != otter.CauseExpiration {
			return
		}

		store.Lock()
		for _, address := range event.Value.Record {
			if val, ok := store.list[address]; !ok {
				continue
			} else if val -= 1; val > 0 {
				store.list[address] = val
				continue
			}

			delete(store.list, address)
			removed = append(removed, address)
		}
		store.Unlock()

		if len(removed) > 0 {
			manager.Broadcast(
				broadcast.UpdateMessage{ToRemove: removed, Cause: broadcast.CauseRemoval},
			)
		}
	}
}

// New creates and initializes a new Repository with the provided logger, broadcaster, and a list of domains.
// Returns the initialized Repository or an error if the initialization fails.
func New(log *logger.Logger, manager broadcast.Broadcaster, domains []string) (Repository, error) {
	var err error
	out := logger.Named(log, serviceName)
	ips := &ipStorage{list: make(map[string]int)}

	var res *otter.Cache[string, Item]
	if res, err = otter.New(&otter.Options[string, Item]{OnDeletion: toRemoveDomains(out, ips, manager)}); err != nil {
		return nil, fmt.Errorf("could not create Domain storage: %w", err)
	}

	for _, domain := range domains {
		res.Set(domain, Item{Domain: domain, ext: make(map[string]time.Time)})
	}

	return &store{
		Logger:  out,
		ipItems: ips,
		domains: res,
		manager: manager,
	}, nil
}
