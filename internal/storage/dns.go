package storage

import (
	"maps"
	"slices"
	"time"

	"github.com/maypok86/otter/v2"

	"github.com/im-kulikov/resolvex/internal/broadcast"
)

// DNS представляет интерфейс для работы с DNS функциональностью.
type DNS interface {
	// Domains используется для DNS, чтобы обработать
	Domains() []string
	// Publish используется для DNS, чтобы обновить записи
	Publish(domains []PublishItem)
}

type PublishItem struct {
	Domain string
	Expire time.Time
	Record map[string]time.Time
}

// Domains return a slice of all domain names currently present in the store, ensuring thread-safe access.
func (s *store) Domains() []string {
	s.ipItems.RLock()
	defer s.ipItems.RUnlock()

	var out []string // nolint:prealloc
	for item := range s.domains.Values() {
		// не нужно обновлять записи, которые не протухли
		if time.Until(item.Expire) > 0 {
			continue
		}

		out = append(out, item.Domain)
	}

	return out
}

// Publish updates the store's domain and IP lists, handling additions, removals, and broadcasting updates.
func (s *store) Publish(domains []PublishItem) {
	s.ipItems.Lock()
	defer s.ipItems.Unlock()

	var msg broadcast.UpdateMessage
	for _, rec := range domains {
		s.domains.Compute(rec.Domain, func(old Item, found bool) (Item, otter.ComputeOp) {
			// если найден в старом списке - удалить из старого списка (для прогона на удаление)
			// если не найден в общем списке - на обновление и +1 к общему
			for address := range rec.Record {
				if _, ok := old.ext[address]; ok {
					delete(old.ext, address)
					continue
				}

				if _, ok := s.ipItems.list[address]; !ok {
					msg.ToUpdate = append(msg.ToUpdate, address)
				}

				s.ipItems.list[address] += 1
			}

			// если не протух - добавляем в новый список
			// если найден в общем и счётчик больше 1 - val-1
			// иначе на удаление
			for address, expires := range old.ext {
				if time.Until(expires) > 0 {
					continue
				} else if val, ok := s.ipItems.list[address]; ok && val > 1 {
					s.ipItems.list[address] -= 1
					continue
				}

				delete(s.ipItems.list, address)
				msg.ToRemove = append(msg.ToRemove, address)
			}

			return Item{
				ext: rec.Record,

				Domain: rec.Domain,
				Expire: rec.Expire,
				Record: slices.Collect(maps.Keys(rec.Record)),
			}, otter.WriteOp
		})
	}

	msg.Cause = broadcast.CauseDNSPublish
	if len(msg.ToUpdate) == 0 && len(msg.ToRemove) == 0 {
		return
	}

	s.manager.Broadcast(msg)
}
