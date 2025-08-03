package storage

import (
	"bytes"
	"encoding/binary"
	"maps"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/im-kulikov/go-bones"
	"github.com/im-kulikov/go-bones/logger"
	"github.com/maypok86/otter/v2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/resolvex/internal/broadcast"
)

type testBroadcaster struct {
	mock.Mock
}

func (t *testBroadcaster) Broadcast(msg broadcast.UpdateMessage) {
	t.Called(msg)
}

func TestStore_Delete(t *testing.T) {
	domains := make([]string, 0, 1)
	manager := new(testBroadcaster)

	log := logger.ForTests(logger.TestLoggerWriteToTB(t))
	svc, err := New(log, manager, domains)
	require.NoError(t, err)
	require.ElementsMatch(t, domains, svc.AllDomains())
	require.ElementsMatch(t, domains, svc.ExpiredDomains())
	require.Empty(t, svc.IPsList())

	require.ErrorIs(t, svc.Delete("google.com"), ErrNotFound)

	manager.On("Broadcast", mock.Anything).Times(3)

	now := time.Now().Add(time.Hour)
	svc.Publish([]PublishItem{{
		Domain: "google.com",
		Expire: now,
		Record: map[string]time.Time{"127.0.0.1": now},
	}})
	svc.Publish([]PublishItem{{
		Domain: "www.google.com",
		Expire: now,
		Record: map[string]time.Time{"127.0.0.1": now},
	}})

	require.NoError(t, svc.Delete("www.google.com"))
	require.ElementsMatch(t, []string{"127.0.0.1"}, svc.IPsList())
	require.ElementsMatch(t, []string{"google.com"}, svc.AllDomains())
}

func TestStore_Update(t *testing.T) {
	domains := make([]string, 0, 1)
	manager := new(testBroadcaster)

	log := logger.ForTests(logger.TestLoggerWriteToTB(t))
	svc, err := New(log, manager, domains)
	require.NoError(t, err)
	require.ElementsMatch(t, domains, svc.AllDomains())
	require.ElementsMatch(t, domains, svc.ExpiredDomains())
	require.Empty(t, svc.IPsList())

	require.ErrorIs(t, svc.Update("google.com", "www.google.com"), ErrNotFound)
}

func TestStore_List(t *testing.T) {
	domains := make([]string, 0, 1)
	manager := new(testBroadcaster)

	log := logger.ForTests(logger.TestLoggerWriteToTB(t))
	svc, err := New(log, manager, domains)
	require.NoError(t, err)
	require.ElementsMatch(t, domains, svc.AllDomains())
	require.ElementsMatch(t, domains, svc.ExpiredDomains())
	require.Empty(t, svc.IPsList())

	require.NoError(t, svc.Create("google.com"))

	for item := range svc.List() {
		require.Empty(t, item.Record)
		require.True(t, time.Until(item.Expire) < 0)
		domains = append(domains, item.Domain)

		break
	}
	require.ElementsMatch(t, domains, svc.ExpiredDomains())
}

func Test_storage(t *testing.T) {
	domains := make([]string, 0, 1)
	manager := new(testBroadcaster)

	domains = append(domains, "www.google.com")

	log := logger.ForTests(logger.TestLoggerWriteToTB(t))
	svc, err := New(log, manager, domains)
	require.NoError(t, err)
	require.ElementsMatch(t, domains, svc.AllDomains())
	require.ElementsMatch(t, domains, svc.ExpiredDomains())
	require.Empty(t, svc.IPsList())

	// проверяем простое удаление
	t.Run("check simple delete", func(t *testing.T) {
		manager.Test(t)

		require.NoError(t, svc.Delete(domains[0]))
		require.Empty(t, svc.AllDomains())
		require.Empty(t, svc.ExpiredDomains())

		manager.AssertExpectations(t)
	})

	// добавляем
	t.Run("check add", func(t *testing.T) {
		manager.Test(t)

		require.NoError(t, svc.Create("google.com"))
		require.Empty(t, svc.IPsList())
		require.ElementsMatch(t, []string{"google.com"}, svc.AllDomains())
		require.ElementsMatch(t, []string{"google.com"}, svc.ExpiredDomains())

		// во второй раз - ошибка
		require.ErrorIs(t, svc.Create("google.com"), ErrExist)

		manager.AssertExpectations(t)
	})

	// publish протухшие адреса
	t.Run("publish expired", func(t *testing.T) {
		manager.Test(t)
		now := time.Now()

		require.NotPanics(t, func() {
			svc.Publish([]PublishItem{
				{Domain: "google.com", Expire: now, Record: map[string]time.Time{
					"127.0.0.1": now,
					"127.0.0.2": now,
				}},
			})
		})
		require.Empty(t, svc.IPsList())
		require.ElementsMatch(t, []string{"google.com"}, svc.AllDomains())
		require.ElementsMatch(t, []string{"google.com"}, svc.ExpiredDomains())

		manager.AssertExpectations(t)
	})

	// publish с корректными данными
	t.Run("publish with correct data", func(t *testing.T) {
		manager.Test(t)
		now := time.Now()
		one := now.Add(time.Hour)

		manager.On("Broadcast",
			broadcast.UpdateMessage{
				Cause:    broadcast.CauseDNSPublish,
				ToUpdate: []string{"127.0.0.1", "127.0.0.2"},
			}).Once()

		require.NotPanics(t, func() {
			svc.Publish([]PublishItem{
				{Domain: "google.com", Expire: one, Record: map[string]time.Time{
					"127.0.0.1": one,
					"127.0.0.2": one,
				}},
			})
		})
		require.ElementsMatch(t, []string{"127.0.0.1", "127.0.0.2"}, svc.IPsList())
		require.Empty(t, svc.ExpiredDomains())
		require.ElementsMatch(t, []string{"google.com"}, svc.AllDomains())

		manager.AssertExpectations(t)
	})

	// обновляем
	t.Run("update", func(t *testing.T) {
		manager.Test(t)
		manager.On("Broadcast",
			broadcast.UpdateMessage{
				Cause:    broadcast.CauseAPIUpdate,
				ToRemove: []string{"127.0.0.1", "127.0.0.2"},
			}).Once()
		require.NoError(t, svc.Update("google.com", "www.google.com"))

		require.Empty(t, svc.IPsList())
		require.ElementsMatch(t, []string{"www.google.com"}, svc.AllDomains())
		require.ElementsMatch(t, []string{"www.google.com"}, svc.ExpiredDomains())

		manager.On("Broadcast",
			broadcast.UpdateMessage{
				Cause:    broadcast.CauseDNSPublish,
				ToUpdate: []string{"127.0.0.2"},
			}).Once()

		now := time.Now().Add(time.Hour)

		require.NotPanics(t, func() {
			svc.Publish([]PublishItem{
				{Domain: "www.google.com", Expire: now, Record: map[string]time.Time{
					"127.0.0.2": now,
				}},
			})
		})

		manager.AssertExpectations(t)
	})

	t.Run("remove", func(t *testing.T) { // удаляем
		manager.Test(t)
		manager.On("Broadcast",
			broadcast.UpdateMessage{
				Cause:    broadcast.CauseAPIDelete,
				ToRemove: []string{"127.0.0.2"},
			}).Once()

		require.NotPanics(t, func() { require.NoError(t, svc.Delete("www.google.com")) })
		require.Empty(t, svc.AllDomains())
		require.Empty(t, svc.ExpiredDomains())
		require.Empty(t, svc.IPsList())

		manager.AssertExpectations(t)
	})

	require.NoError(t, svc.(*store).validate("TEST"))

	manager.Test(t)
	manager.AssertExpectations(t)
}

func Test_shouldFail(t *testing.T) {
	domains := []string{"google.com"}
	manager := new(testBroadcaster)
	manager.Test(t)

	log := logger.ForTests(logger.TestLoggerWriteToTB(t))
	require.Error(
		t,
		bones.ExtractError(New(log, manager, domains, func(o *otter.Options[string, Item]) {
			o.MaximumSize = 1
			o.MaximumWeight = 1
		})),
	)
}

func getOutdated(ips map[string]time.Time) []string {
	out := make([]string, 0, len(ips))
	for address, expires := range ips {
		if time.Until(expires) > 0 {
			continue
		}

		out = append(out, address)
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

func Test_publishSuccess(t *testing.T) {
	domains := make([]string, 0)
	manager := new(testBroadcaster)
	manager.Test(t)

	buf := new(bytes.Buffer)
	log := logger.ForTests(logger.TestLoggerWriteToTB(t), logger.TestLoggerWriter(buf))
	svc, err := New(log, manager, domains)
	require.NoError(t, err)

	require.NoError(t, svc.Create("google.com"))
	require.ElementsMatch(t, []string{"google.com"}, svc.AllDomains())
	require.ElementsMatch(t, []string{"google.com"}, svc.ExpiredDomains())

	now := time.Now()
	ips := make(map[string]time.Time)
	for i := range 100 {
		out := make([]string, 0, 100*i)
		lst := make(map[string]time.Time)
		for j := range 10 {
			for k := range 5 {
				one := now.Add(time.Second * 2)
				num := 127<<24 + i<<16 + k<<4 + j + 1
				adr := net.IP(binary.BigEndian.AppendUint32(nil, uint32(num))).String()
				if _, ok := lst[adr]; ok {
					continue
				}

				out = append(out, adr)

				lst[adr] = one
				ips[adr] = one
			}
		}

		if updates, removes := out, getOutdated(ips); len(updates) > 0 || len(removes) > 0 {
			slices.Sort(updates)
			slices.Sort(removes)

			manager.On("Broadcast",
				broadcast.UpdateMessage{
					Cause:    broadcast.CauseDNSPublish,
					ToUpdate: updates,
					ToRemove: removes,
				}).Once()
		}

		require.NotPanics(t, func() {
			svc.Publish([]PublishItem{{
				Record: lst,
				Domain: "google.com",
				Expire: now.Add(time.Hour),
			}})
		})

		maps.DeleteFunc(ips, func(_ string, expires time.Time) bool {
			return time.Until(expires) <= 0
		})
	}

	time.Sleep(time.Second*2 - time.Since(now))
	out := []string{"255.0.0.1"}
	if updates, removes := out, getOutdated(ips); len(updates) > 0 || len(removes) > 0 {
		slices.Sort(updates)
		slices.Sort(removes)

		manager.On("Broadcast",
			broadcast.UpdateMessage{
				Cause:    broadcast.CauseDNSPublish,
				ToUpdate: updates,
				ToRemove: removes,
			}).Once()
	}

	svc.Publish([]PublishItem{{
		Domain: "google.com",
		Expire: now.Add(time.Hour),
		Record: map[string]time.Time{"255.0.0.1": time.Now().Add(time.Hour)},
	}})

	svc.Publish([]PublishItem{{
		Domain: "google.com",
		Expire: now.Add(time.Hour),
		Record: map[string]time.Time{"255.0.0.1": time.Now().Add(time.Hour)},
	}})

	svc.Publish([]PublishItem{{
		Domain: "www.google.com",
		Expire: now.Add(time.Hour),
		Record: map[string]time.Time{"255.0.0.1": time.Now().Add(time.Millisecond * 100)},
	}})

	time.Sleep(time.Millisecond * 100)
	manager.On("Broadcast",
		broadcast.UpdateMessage{
			Cause:    broadcast.CauseDNSPublish,
			ToUpdate: []string{"255.0.0.2"},
		}).Once()

	svc.Publish([]PublishItem{{
		Domain: "www.google.com",
		Expire: now.Add(time.Hour),
		Record: map[string]time.Time{"255.0.0.2": time.Now().Add(time.Hour)},
	}})

	require.NoError(t, svc.(*store).validate("test"))
	manager.AssertExpectations(t)
}
