package storage

import (
	"fmt"
	"iter"
	"slices"

	"github.com/im-kulikov/go-bones/logger"
	"github.com/maypok86/otter/v2"

	"github.com/im-kulikov/resolvex/internal/broadcast"
)

// API defines an interface for managing domain operations such as creation, deletion, updating, and listing.
type API interface {
	// Create используется в API, чтобы добавить новый домен
	Create(domain string) error
	// Delete используется в API, чтобы удалить существующий домен
	Delete(domain string) error
	// Update используется в API, чтобы изменить доменное имя
	Update(oldDomain, newDomain string) error
	// List используется в API, чтобы отобразить список доменов и адресов
	List() iter.Seq[Item]
}

// Create add a new domain to the store if it does not already exist, returning an error if the domain exists.
func (s *store) Create(domain string) error {
	s.ipItems.RLock()
	defer s.ipItems.RUnlock()

	var err error
	s.domains.Compute(domain, func(oldValue Item, found bool) (Item, otter.ComputeOp) {
		if found {
			err = fmt.Errorf("%w: %s", ErrExist, domain)

			return Item{}, otter.CancelOp
		}

		return Item{Domain: domain}, otter.WriteOp
	})
	if err != nil {
		return err
	}

	if err = s.validate("Create"); err != nil {
		s.Error("validate failed", logger.Err(err))
	}

	return nil
}

// Delete removes the specified domain from the store along with associated IPs, broadcasting updates for removed IPs.
func (s *store) Delete(domain string) error {
	s.ipItems.Lock()
	defer s.ipItems.Unlock()

	err, msg := (error)(nil), broadcast.UpdateMessage{Cause: broadcast.CauseAPIDelete}
	s.domains.Compute(domain, func(old Item, found bool) (Item, otter.ComputeOp) {
		if !found {
			err = fmt.Errorf("%w: %s", ErrNotFound, domain)

			return old, otter.CancelOp
		}

		// собираем список на удаление
		for address := range old.ext {
			// если в общем есть, но количество меньше или равно 1 - удаляем
			val, ok := s.ipItems.list[address]
			if ok && val <= 1 {
				msg.ToRemove = append(msg.ToRemove, address)

				delete(s.ipItems.list, address)

				continue
			}

			// иначе - уменьшаем на единицу
			s.ipItems.list[address] -= 1
		}

		// указываем, что необходимо удалить запись
		return old, otter.InvalidateOp
	})

	if err != nil {
		return err
	}

	if len(msg.ToRemove) != 0 {
		slices.Sort(msg.ToUpdate)
		slices.Sort(msg.ToRemove)
		s.manager.Broadcast(msg)
	}

	if err = s.validate("Delete"); err != nil {
		s.Error("validate failed", logger.Err(err))
	}

	return nil
}

// Update modifies an existing domain to a new domain, ensuring the new domain does not already exist in the store.
// Returns an error if the old domain does not exist or the new domain already exists.
// Handles removal and decrement of associated IP addresses and broadcast removals if necessary.
func (s *store) Update(oldDomain, newDomain string) error {
	s.ipItems.Lock()
	defer s.ipItems.Unlock()

	if _, ok := s.domains.GetEntry(newDomain); ok {
		return fmt.Errorf("could not change %q to %q: %w", oldDomain, newDomain, ErrExist)
	}

	err, msg := (error)(nil), broadcast.UpdateMessage{Cause: broadcast.CauseAPIUpdate}
	s.domains.Compute(oldDomain, func(old Item, found bool) (Item, otter.ComputeOp) {
		if !found {
			err = fmt.Errorf("could not change %q to %q: %w", oldDomain, newDomain, ErrNotFound)

			return old, otter.CancelOp
		}

		// собираем список на удаление
		for address := range old.ext {
			// если в общем есть, но количество меньше или равно 1 - удаляем
			if val, ok := s.ipItems.list[address]; ok && val <= 1 {
				msg.ToRemove = append(msg.ToRemove, address)

				delete(s.ipItems.list, address)

				continue
			}

			// иначе - уменьшаем на единицу
			s.ipItems.list[address] -= 1
		}

		// указываем, что необходимо удалить запись
		return old, otter.InvalidateOp
	})

	if err != nil {
		return err
	}

	s.domains.Compute(newDomain, func(oldValue Item, found bool) (Item, otter.ComputeOp) {
		if found {
			err = fmt.Errorf("%w: %s", ErrExist, newDomain)

			return Item{}, otter.CancelOp
		}

		return Item{Domain: newDomain}, otter.WriteOp
	})

	if err != nil {
		return err
	}

	if len(msg.ToRemove) != 0 {
		slices.Sort(msg.ToUpdate)
		slices.Sort(msg.ToRemove)
		s.manager.Broadcast(msg)
	}

	if err = s.validate("Update"); err != nil {
		s.Error("validate failed", logger.Err(err))
	}

	return nil
}

// List returns a map where keys are domain names and values are lists of active IP addresses
// associated with each domain.
func (s *store) List() iter.Seq[Item] {
	s.ipItems.RLock()
	defer s.ipItems.RUnlock()

	return func(yield func(Item) bool) {
		for rec := range s.domains.Values() {
			if !yield(
				Item{Domain: rec.Domain, Expire: rec.Expire, Record: slices.Clone(rec.Record)},
			) {
				return
			}
		}
	}
}
