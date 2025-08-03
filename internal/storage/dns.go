package storage

import (
	"maps"
	"slices"
	"time"

	"github.com/im-kulikov/go-bones/logger"
	"github.com/maypok86/otter/v2"

	"github.com/im-kulikov/resolvex/internal/broadcast"
)

// DNS представляет интерфейс для работы с DNS функциональностью.
type DNS interface {
	// AllDomains получить список всех доменов
	AllDomains() []string
	// ExpiredDomains используется для DNS, чтобы обработать
	ExpiredDomains() []string
	// Publish используется для DNS, чтобы обновить записи
	Publish(domains []PublishItem)
}

type PublishItem struct {
	Domain string
	Expire time.Time
	Record map[string]time.Time
}

func (s *store) getDomains(expired bool) []string {
	var out []string // nolint:prealloc
	for item := range s.domains.Values() {
		// не нужно обновлять записи, которые не протухли
		if expired && time.Until(item.Expire) > 0 {
			continue
		}

		out = append(out, item.Domain)
	}

	return out
}

func (s *store) AllDomains() []string {
	s.ipItems.RLock()
	defer s.ipItems.RUnlock()

	return s.getDomains(false)
}

// ExpiredDomains return a slice of all domain names currently present in the store, ensuring thread-safe access.
func (s *store) ExpiredDomains() []string {
	s.ipItems.RLock()
	defer s.ipItems.RUnlock()

	return s.getDomains(true)
}

// Publish updates the store's domain and IP lists, handling additions, removals, and broadcasting updates.
func (s *store) Publish(domains []PublishItem) {
	s.ipItems.Lock()
	defer s.ipItems.Unlock()

	var msg broadcast.UpdateMessage
	// Идём по новым доменам
	for _, rec := range domains {
		// обрабатываем каждую запись
		s.domains.Compute(rec.Domain, func(old Item, found bool) (Item, otter.ComputeOp) {
			// сначала очищаем от старых записей и формируем список обновлений
			//   - если запись из нового списка протухшая - пропускаем / continue
			//   - если запись из нового списка уже есть в старом — запоминаем, что есть более новая версия и continue
			//   - если нет записи в общем счётчике - добавляем в список обновления
			//   - инкремент общего счётчика
			has := make(map[string]struct{})
			lst := make(map[string]time.Time)
			for address, expires := range rec.Record {
				if time.Until(expires) <= 0 {
					continue
				}

				if _, ok := old.ext[address]; ok {
					lst[address] = expires
					has[address] = struct{}{}
					continue
				}

				if _, ok := s.ipItems.list[address]; !ok {
					msg.ToUpdate = append(msg.ToUpdate, address)
				}

				lst[address] = expires
				s.ipItems.list[address] += 1
			}

			// теперь нужно пройтись по тем записям, что остались в старом списке и не были найдены в новом
			//   - если запись есть в новом списке - пропускаем, continue
			//   - если не протух - добавляем в новый список и continue
			//   - если найден в общем списке и счётчик больше 1 - просто декремент и continue
			//   - иначе удаляем из общего списка и добавляем в список на удаление
			// иначе на удаление
			for address, expires := range old.ext {
				if _, ok := has[address]; ok {
					continue
				}

				if time.Until(expires) > 0 {
					lst[address] = expires
					continue
				}

				if val, ok := s.ipItems.list[address]; ok && val > 1 {
					lst[address] = expires
					s.ipItems.list[address] -= 1
					continue
				}

				delete(s.ipItems.list, address)
				msg.ToRemove = append(msg.ToRemove, address)
			}

			// по завершению - сохраняем новый элемент
			return Item{
				ext: lst,

				Domain: rec.Domain,
				Expire: rec.Expire,
				Record: slices.Collect(maps.Keys(lst)),
			}, otter.WriteOp
		})
	}

	msg.Cause = broadcast.CauseDNSPublish

	// если есть обновления - отправляем
	if len(msg.ToUpdate) > 0 || len(msg.ToRemove) > 0 {
		slices.Sort(msg.ToUpdate)
		slices.Sort(msg.ToRemove)
		s.manager.Broadcast(msg)
	}

	if err := s.validate("CauseDNSPublish"); err != nil {
		s.Error("validate failed", logger.Err(err))
	}
}
