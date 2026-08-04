package session

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func TestQueueAdmitsOldestRequestWhenCapacityOpens(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	manager := NewManager(client, zap.NewNop())
	manager.maxSessions = 1
	ctx := context.Background()

	first, queue, err := manager.Create(ctx, "first", "")
	if err != nil || queue != nil {
		t.Fatalf("create first session: queue=%v err=%v", queue, err)
	}

	_, queue, err = manager.Create(ctx, "second", "")
	if err != nil {
		t.Fatalf("queue second session: %v", err)
	}
	if queue == nil || !queue.Queued || queue.Position != 1 || queue.Token == "" {
		t.Fatalf("unexpected queue response: %#v", queue)
	}

	if err := manager.MarkTerminated(ctx, first.ID); err != nil {
		t.Fatalf("terminate first session: %v", err)
	}
	second, queue, err := manager.Create(ctx, "second", queue.Token)
	if err != nil || queue != nil || second == nil {
		t.Fatalf("admit queued session: session=%#v queue=%#v err=%v", second, queue, err)
	}
}

func TestDisconnectCleanupCanBeCancelled(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	manager := NewManager(client, zap.NewNop())

	cleaned := make(chan struct{}, 1)
	manager.ScheduleDisconnect("session", 20*time.Millisecond, func() {
		cleaned <- struct{}{}
	})
	manager.CancelDisconnect("session")

	select {
	case <-cleaned:
		t.Fatal("cleanup ran after reconnect cancellation")
	case <-time.After(40 * time.Millisecond):
	}
}
