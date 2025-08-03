package storage

import (
	"fmt"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
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

type Option func(options *otter.Options[string, Item])

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
	ips *ipStorage,
	// manager broadcast.Broadcaster,
) cleanDomainListener {
	return func(event otter.DeletionEvent[string, Item]) {
		ips.Lock()
		defer ips.Unlock()

		log.Debug("record was replaced or removed",
			logger.String("domain", event.Key),
			logger.String("cause", event.Cause.String()))

		if event.Cause != otter.CauseExpiration {
			return
		}

		panic(event.Cause)

		// var removed []string
		// for _, address := range event.Value.Record {
		// 	if val, ok := ips.list[address]; !ok {
		// 		continue
		// 	} else if val -= 1; val > 0 {
		// 		ips.list[address] = val
		// 		continue
		// 	}
		//
		// 	delete(ips.list, address)
		// 	removed = append(removed, address)
		// }
		//
		// if len(removed) > 0 {
		// 	slices.Sort(removed)
		// 	manager.Broadcast(broadcast.UpdateMessage{ToRemove: removed, Cause: broadcast.CauseRemoval})
		// }
		//
		// (&store{ipItems: ips}).validate(toRemoveDomains)
	}
}

// New creates and initializes a new Repository with the provided logger, broadcaster, and a list of domains.
// Returns the initialized Repository or an error if the initialization fails.
func New(
	log *logger.Logger,
	manager broadcast.Broadcaster,
	domains []string,
	options ...Option,
) (Repository, error) {
	var err error
	out := logger.Named(log, serviceName)
	ips := &ipStorage{list: make(map[string]int)}

	var opts otter.Options[string, Item]
	for _, o := range options {
		o(&opts)
	}

	// rewrite a deletion function
	opts.OnDeletion = toRemoveDomains(out, ips)

	var res *otter.Cache[string, Item]
	if res, err = otter.New(&opts); err != nil {
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

func (s *store) validate(where any) error {
	list := make(map[string]struct{})
	lost := make(map[string]struct{})
	for _, address := range s.getIPList() {
		list[address] = struct{}{}
		lost[address] = struct{}{}
	}

	find := make(map[string]struct{})
	for item := range s.domains.Values() {
		for address := range item.ext {
			if _, ok := list[address]; ok {
				delete(lost, address)
			} else {
				find[address] = struct{}{}
			}
		}
	}

	if len(lost) > 0 || len(find) > 0 {
		return fmt.Errorf("found problem => Cause: %v, Lost: %s, Find: %s",
			where, spew.Sdump(lost), spew.Sdump(find))
	}

	return nil
}
