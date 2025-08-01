package storage

// BGP представляет интерфейс для работы с BGP функциональностью.
// IPsList возвращает список IP-адресов, который используется для отправки начальной таблицы.
type BGP interface {
	// IPsList используется для BGP, чтобы отправить начальную таблицу
	IPsList() []string
}

// IPsList retrieves a list of IP addresses from the store with positive counters, ensuring thread-safe access.
func (s *store) IPsList() []string {
	s.ipItems.RLock()
	defer s.ipItems.RUnlock()

	result := make([]string, 0, len(s.ipItems.list))
	for address, counter := range s.ipItems.list {
		if counter <= 0 {
			continue
		}

		result = append(result, address)
	}

	return result
}
