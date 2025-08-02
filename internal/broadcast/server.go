package broadcast

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
)

type server struct {
	sync.RWMutex
	*logger.Logger
	service.Service

	config Config
	closed *atomic.Bool
	action chan updatePeer
	output chan UpdateMessage
}

// Service represents an interface that combines Broadcaster, PeerManager, and the base service.Service functionalities.
type Service interface {
	Broadcaster
	PeerManager
	service.Service
}

// PeerWriter defines a function type for sending an UpdateMessage to a peer with a context
// for cancellation and deadlines.
type PeerWriter func(ctx context.Context, message UpdateMessage) error

type Config struct {
	Interval time.Duration `env:"INTERVAL" default:"90s"`
}

type actionType uint8

const (
	_ actionType = iota
	addPeer
	remPeer
)

// New creates and initializes a new Service instance with the provided configuration and logger.
func New(cfg Config, log *logger.Logger) Service {
	closed := new(atomic.Bool)
	action := make(chan updatePeer, 10)
	output := make(chan UpdateMessage, 10)

	out := logger.Named(log, "broadcaster")

	return &server{
		config: cfg,
		action: action,
		output: output,
		closed: closed,

		Logger: out,
		Service: service.NewLauncher("broadcaster",
			runner(out, runnerParams{closed: closed, action: action, output: output}),
			func(ctx context.Context) { out.InfoContext(ctx, "shutdown gracefully") }),
	}
}

type runnerParams struct {
	closed *atomic.Bool
	action chan updatePeer
	output chan UpdateMessage
}

func runner(log *logger.Logger, rp runnerParams) service.Launcher {
	return func(ctx context.Context) error {
		var (
			list []string
			peer = make(map[string]PeerWriter)
		)

		ticker := time.NewTimer(time.Millisecond)
		defer ticker.Stop()

		log.InfoContext(ctx, "prepare")

	loop:
		for {
			select {
			case <-ctx.Done():
				log.InfoContext(ctx, "try shutdown")

				rp.closed.Store(true)
				close(rp.action)
				close(rp.output)

				return nil

			case msg := <-rp.action:
				switch msg.Action {
				case addPeer:
					log.InfoContext(ctx, "try update peer",
						logger.String("peer", msg.Peer))

					if _, ok := peer[msg.Peer]; ok {
						log.InfoContext(ctx, "update exists peer",
							logger.String("peer", msg.Peer),
							logger.Int("updates", len(list)))

						peer[msg.Peer] = msg.writer

						continue loop
					}

					peer[msg.Peer] = msg.writer

					log.InfoContext(ctx, "try to send initial table",
						logger.String("peer", msg.Peer),
						logger.Int("updates", len(list)))

					if err := msg.writer(ctx, UpdateMessage{ToUpdate: list}); err != nil {
						log.ErrorContext(ctx, "could not send initial table", logger.Err(err))
					}

					log.InfoContext(ctx, "current peers", logger.Int("count", len(peer)))

				case remPeer:
					log.InfoContext(ctx, "remove peer writer", logger.String("peer", msg.Peer))
					delete(peer, msg.Peer)
				default:
					log.ErrorContext(ctx, "unknown Action", logger.Any("Action", msg))
				}

			case msg := <-rp.output:
				if len(msg.ToUpdate) == 0 && len(msg.ToRemove) == 0 {
					log.InfoContext(ctx, "ignore empty message update",
						logger.String("cause", msg.Cause.String()))

					continue loop
				}

				list = updateList(log, list, msg)

				log.InfoContext(ctx, "would sent to peers",
					logger.Int("peers", len(peer)),
					logger.Int("update-count", len(msg.ToUpdate)),
					logger.Int("remove-count", len(msg.ToRemove)),
					logger.Any("update-list", msg.ToUpdate),
					logger.Any("remove-list", msg.ToRemove))

				for name, writer := range peer {
					if err := writer(ctx, msg); err != nil {
						log.ErrorContext(ctx, "could not send update table", logger.Err(err))
						continue loop
					}

					log.InfoContext(ctx, "message send successfully",
						logger.String("peer", name),
						logger.Int("update-count", len(msg.ToUpdate)),
						logger.Int("remove-count", len(msg.ToRemove)),
						logger.Any("update-list", msg.ToUpdate),
						logger.Any("remove-list", msg.ToRemove))
				}
			}
		}
	}
}

func updateList(log *logger.Logger, list []string, msg UpdateMessage) []string {
	if len(msg.ToUpdate) == 0 && len(msg.ToRemove) == 0 {
		return list
	}

	log.Info("before update",
		logger.Int("msg.update", len(msg.ToUpdate)),
		logger.Int("msg.remove", len(msg.ToRemove)),
		logger.Int("list", len(list)))

	// Создаем карту для отслеживания элементов списка
	listMap := make(map[string]bool)
	for _, item := range list {
		listMap[item] = true
	}

	// Удаляем элементы, которые должны быть удалены
	for _, item := range msg.ToRemove {
		delete(listMap, item)
	}

	// Добавляем новые элементы
	for _, item := range msg.ToUpdate {
		listMap[item] = true
	}

	// Создаем обновленный список из карты
	updatedList := make([]string, 0, len(listMap))
	for item := range listMap {
		updatedList = append(updatedList, item)
	}

	log.Info("after update",
		logger.Int("msg.update", len(msg.ToUpdate)),
		logger.Int("msg.remove", len(msg.ToRemove)),
		logger.Int("list", len(updatedList)))

	return updatedList
}
