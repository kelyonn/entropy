package session

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type Status string

const (
	StatusCreating    Status = "CREATING"
	StatusActive      Status = "ACTIVE"
	StatusIdle        Status = "IDLE"
	StatusTerminating Status = "TERMINATING"
	StatusTerminated  Status = "TERMINATED"
)

type Session struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id,omitempty"`
	ContainerID  string    `json:"container_id,omitempty"`
	Status       Status    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	LastActivity time.Time `json:"last_activity"`
}

type Manager struct {
	rdb          *redis.Client
	logger       *zap.Logger
	maxSessions  int
	idleTimeout  time.Duration
	maxLifetime  time.Duration
	mu           sync.Mutex
	onTerminate  func(ctx context.Context, s Session, reason string)
	disconnectMu sync.Mutex
	disconnects  map[string]context.CancelFunc
}

func NewManager(rdb *redis.Client, logger *zap.Logger) *Manager {
	max := envInt("MAX_SESSIONS", 100)
	idleMin := envInt("IDLE_TIMEOUT_MINUTES", 5)
	maxHours := envInt("MAX_SESSION_HOURS", 1)
	idleTimeout := time.Duration(idleMin) * time.Minute
	if idleSeconds := envInt("IDLE_TIMEOUT_SECONDS", 0); idleSeconds > 0 {
		idleTimeout = time.Duration(idleSeconds) * time.Second
	}
	maxLifetime := time.Duration(maxHours) * time.Hour
	if lifetimeSeconds := envInt("MAX_SESSION_SECONDS", 0); lifetimeSeconds > 0 {
		maxLifetime = time.Duration(lifetimeSeconds) * time.Second
	}
	return &Manager{
		rdb:         rdb,
		logger:      logger,
		maxSessions: max,
		idleTimeout: idleTimeout,
		maxLifetime: maxLifetime,
		disconnects: map[string]context.CancelFunc{},
	}
}

func (m *Manager) SetTerminateHook(fn func(ctx context.Context, s Session, reason string)) {
	m.onTerminate = fn
}

type QueueInfo struct {
	Queued           bool   `json:"queued"`
	Token            string `json:"queue_token,omitempty"`
	Position         int    `json:"position,omitempty"`
	EstimatedWaitSec int    `json:"estimated_wait_sec,omitempty"`
}

func (m *Manager) Create(ctx context.Context, userID, queueToken string) (*Session, *QueueInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	active, err := m.countActive(ctx)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	m.rdb.ZRemRangeByScore(ctx, "session:queue", "0", strconv.FormatInt(now.Add(-5*time.Minute).UnixNano(), 10))
	queueSize, err := m.rdb.ZCard(ctx, "session:queue").Result()
	if err != nil {
		return nil, nil, err
	}

	if active >= m.maxSessions || queueSize > 0 || queueToken != "" {
		if queueToken == "" {
			queueToken = uuid.New().String()
		}
		if err := m.rdb.ZAddNX(ctx, "session:queue", redis.Z{
			Score: float64(now.UnixNano()), Member: queueToken,
		}).Err(); err != nil {
			return nil, nil, err
		}
		if depth, depthErr := m.rdb.ZCard(ctx, "session:queue").Result(); depthErr == nil {
			m.rdb.Set(ctx, "metric:queue_depth", depth, 0)
		}
		rank, err := m.rdb.ZRank(ctx, "session:queue", queueToken).Result()
		if err != nil {
			return nil, nil, err
		}
		if active >= m.maxSessions || rank > 0 {
			position := int(rank) + 1
			return nil, &QueueInfo{
				Queued:           true,
				Token:            queueToken,
				Position:         position,
				EstimatedWaitSec: position * 30,
			}, nil
		}
		m.rdb.ZRem(ctx, "session:queue", queueToken)
		if depth, depthErr := m.rdb.ZCard(ctx, "session:queue").Result(); depthErr == nil {
			m.rdb.Set(ctx, "metric:queue_depth", depth, 0)
		}
	}

	s := &Session{
		ID:           uuid.New().String(),
		UserID:       userID,
		Status:       StatusCreating,
		CreatedAt:    time.Now().UTC(),
		LastActivity: time.Now().UTC(),
	}
	if err := m.save(ctx, s); err != nil {
		return nil, nil, err
	}
	m.logger.Info("session created",
		zap.String("session_id", s.ID),
		zap.Bool("authenticated", userID != ""))
	m.rdb.Incr(ctx, "metric:total_sessions")
	if userID != "" {
		m.rdb.SAdd(ctx, "user:sessions:"+userID, s.ID)
	}
	return s, nil, nil
}

func (m *Manager) CancelDisconnect(sessionID string) {
	m.disconnectMu.Lock()
	defer m.disconnectMu.Unlock()
	if cancel, ok := m.disconnects[sessionID]; ok {
		cancel()
		delete(m.disconnects, sessionID)
	}
}

func (m *Manager) ScheduleDisconnect(sessionID string, grace time.Duration, cleanup func()) {
	m.CancelDisconnect(sessionID)
	ctx, cancel := context.WithCancel(context.Background())
	m.disconnectMu.Lock()
	m.disconnects[sessionID] = cancel
	m.disconnectMu.Unlock()
	go func() {
		timer := time.NewTimer(grace)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			cleanup()
			m.disconnectMu.Lock()
			delete(m.disconnects, sessionID)
			m.disconnectMu.Unlock()
		}
	}()
}

func (m *Manager) Activate(ctx context.Context, id, containerID string) error {
	s, err := m.Get(ctx, id)
	if err != nil {
		return err
	}
	s.ContainerID = containerID
	s.Status = StatusActive
	s.LastActivity = time.Now().UTC()
	return m.save(ctx, s)
}

func (m *Manager) RegisterRestored(ctx context.Context, id, userID, containerID string) error {
	now := time.Now().UTC()
	s := &Session{
		ID:           id,
		UserID:       userID,
		ContainerID:  containerID,
		Status:       StatusActive,
		CreatedAt:    now,
		LastActivity: now,
	}
	if err := m.save(ctx, s); err != nil {
		return err
	}
	m.logger.Info("restored session registered", zap.String("session_id", id))
	m.rdb.Incr(ctx, "metric:total_sessions")
	if userID != "" {
		m.rdb.SAdd(ctx, "user:sessions:"+userID, id)
	}
	return nil
}

func (m *Manager) Touch(ctx context.Context, id string) error {
	s, err := m.Get(ctx, id)
	if err != nil {
		return err
	}
	s.LastActivity = time.Now().UTC()
	if s.Status == StatusIdle {
		s.Status = StatusActive
	}
	return m.save(ctx, s)
}

func (m *Manager) Get(ctx context.Context, id string) (*Session, error) {
	raw, err := m.rdb.Get(ctx, "session:"+id).Result()
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (m *Manager) ListForUser(ctx context.Context, userID string) ([]Session, error) {
	ids, err := m.rdb.SMembers(ctx, "user:sessions:"+userID).Result()
	if err != nil {
		return nil, err
	}
	out := make([]Session, 0, len(ids))
	for _, id := range ids {
		s, err := m.Get(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, *s)
	}
	return out, nil
}

func (m *Manager) MarkTerminating(ctx context.Context, id string) (*Session, error) {
	s, err := m.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	s.Status = StatusTerminating
	if err := m.save(ctx, s); err != nil {
		return nil, err
	}
	return s, nil
}

func (m *Manager) MarkTerminated(ctx context.Context, id string) error {
	s, err := m.Get(ctx, id)
	if err != nil {
		return err
	}
	s.Status = StatusTerminated
	return m.save(ctx, s)
}

func (m *Manager) StartSweeper(ctx context.Context) {
	go func() {
		interval := time.Duration(envInt("SESSION_SWEEP_SECONDS", 30)) * time.Second
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				m.sweep(ctx)
			}
		}
	}()
}

func (m *Manager) sweep(ctx context.Context) {
	keys, err := m.rdb.Keys(ctx, "session:*").Result()
	if err != nil {
		return
	}
	now := time.Now().UTC()
	for _, key := range keys {
		raw, err := m.rdb.Get(ctx, key).Result()
		if err != nil {
			continue
		}
		var s Session
		if json.Unmarshal([]byte(raw), &s) != nil {
			continue
		}
		if s.Status != StatusActive && s.Status != StatusIdle && s.Status != StatusCreating {
			continue
		}
		idle := now.Sub(s.LastActivity) > m.idleTimeout
		expired := now.Sub(s.CreatedAt) > m.maxLifetime
		if idle || expired {
			if idle && !expired {
				s.Status = StatusIdle
				_ = m.save(ctx, &s)
			}
			m.logger.Info("terminating session",
				zap.String("session_id", s.ID),
				zap.Bool("idle", idle),
				zap.Bool("expired", expired))
			s2, err := m.MarkTerminating(ctx, s.ID)
			if err != nil {
				continue
			}
			reason := "maximum lifetime reached"
			if idle {
				reason = "idle timeout"
			}
			if m.onTerminate != nil {
				m.onTerminate(ctx, *s2, reason)
			}
			_ = m.MarkTerminated(ctx, s.ID)
		}
	}
}

func (m *Manager) save(ctx context.Context, s *Session) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return m.rdb.Set(ctx, "session:"+s.ID, data, 30*24*time.Hour).Err()
}

func (m *Manager) countActive(ctx context.Context) (int, error) {
	keys, err := m.rdb.Keys(ctx, "session:*").Result()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, key := range keys {
		raw, err := m.rdb.Get(ctx, key).Result()
		if err != nil {
			continue
		}
		var s Session
		if json.Unmarshal([]byte(raw), &s) != nil {
			continue
		}
		if s.Status == StatusActive || s.Status == StatusCreating || s.Status == StatusIdle {
			n++
		}
	}
	return n, nil
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
