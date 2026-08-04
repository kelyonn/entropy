package db

import (
	"context"
	"database/sql"
	"os"
	"time"

	"github.com/entropy/pkg/resilience"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

type Store struct {
	DB      *sql.DB
	logger  *zap.Logger
	breaker *resilience.CircuitBreaker
}

func Connect(logger *zap.Logger) (*Store, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		logger.Info("DATABASE_URL unset; Postgres disabled")
		return &Store{logger: logger}, nil
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		logger.Warn("Postgres ping failed; continuing without DB", zap.Error(err))
		return &Store{logger: logger}, nil
	}
	s := &Store{DB: db, logger: logger, breaker: resilience.NewCircuitBreaker(5, 30*time.Second)}
	if err := s.Migrate(ctx); err != nil {
		logger.Warn("migrate failed", zap.Error(err))
	}
	return s, nil
}

func (s *Store) Enabled() bool {
	return s != nil && s.DB != nil
}

func (s *Store) Migrate(ctx context.Context) error {
	if !s.Enabled() {
		return nil
	}
	_, err := s.exec(ctx, `
CREATE TABLE IF NOT EXISTS sessions (
  id UUID PRIMARY KEY,
  user_id VARCHAR(255),
  container_id VARCHAR(64),
  status VARCHAR(20) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_activity TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  terminated_at TIMESTAMPTZ,
  commands_executed INT DEFAULT 0,
  crashes_encountered INT DEFAULT 0
);
CREATE TABLE IF NOT EXISTS commands (
  id BIGSERIAL PRIMARY KEY,
  session_id UUID NOT NULL,
  command TEXT NOT NULL,
  exit_code INT,
  stdout TEXT,
  stderr TEXT,
  executed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  duration_ms INT
);
CREATE TABLE IF NOT EXISTS kernel_events (
  id BIGSERIAL PRIMARY KEY,
  session_id UUID NOT NULL,
  event_type VARCHAR(50) NOT NULL,
  pid INT,
  signal INT,
  exit_code INT,
  event_data JSONB,
  detected_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS snapshots (
  id UUID PRIMARY KEY,
  session_id UUID NOT NULL,
  s3_bucket VARCHAR(255) NOT NULL,
  s3_key VARCHAR(512) NOT NULL,
  size_bytes BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  expires_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS ai_explanations (
  id BIGSERIAL PRIMARY KEY,
  session_id UUID NOT NULL,
  explanation TEXT NOT NULL,
  model VARCHAR(100) NOT NULL,
  tokens_used INT DEFAULT 0,
  generated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`)
	return err
}

func (s *Store) UpsertSession(ctx context.Context, id, userID, containerID, status string) error {
	if !s.Enabled() {
		return nil
	}
	_, err := s.exec(ctx, `
INSERT INTO sessions (id, user_id, container_id, status)
VALUES ($1,$2,$3,$4)
ON CONFLICT (id) DO UPDATE SET
  container_id = EXCLUDED.container_id,
  status = EXCLUDED.status,
  last_activity = NOW()
`, id, nullStr(userID), containerID, status)
	return err
}

func (s *Store) InsertCommand(ctx context.Context, sessionID, command string, exitCode int, stdout, stderr string) error {
	if !s.Enabled() {
		return nil
	}
	_, err := s.exec(ctx, `
INSERT INTO commands (session_id, command, exit_code, stdout, stderr)
VALUES ($1,$2,$3,$4,$5)
`, sessionID, command, exitCode, truncate(stdout, 10000), truncate(stderr, 10000))
	return err
}

func (s *Store) exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	if s.breaker != nil && !s.breaker.Allow() {
		s.logger.Warn("Postgres circuit open")
		return nil, sql.ErrConnDone
	}
	res, err := s.DB.ExecContext(ctx, query, args...)
	if err != nil {
		if s.breaker != nil {
			s.breaker.RecordFailure()
		}
		return nil, err
	}
	if s.breaker != nil {
		s.breaker.RecordSuccess()
	}
	return res, nil
}

func (s *Store) InsertSnapshot(ctx context.Context, id, sessionID, bucket, key string, size int64, expires time.Time) error {
	if !s.Enabled() {
		return nil
	}
	_, err := s.exec(ctx, `
INSERT INTO snapshots (id, session_id, s3_bucket, s3_key, size_bytes, expires_at)
VALUES ($1,$2,$3,$4,$5,$6)
`, id, sessionID, bucket, key, size, expires)
	return err
}

func (s *Store) FinalizeSession(ctx context.Context, id string, commands, crashes int) error {
	if !s.Enabled() {
		return nil
	}
	_, err := s.exec(ctx, `
UPDATE sessions SET
  status = 'TERMINATED',
  terminated_at = NOW(),
  commands_executed = $2,
  crashes_encountered = $3
WHERE id = $1
`, id, commands, crashes)
	return err
}

func (s *Store) ListCommands(ctx context.Context, sessionID string, limit int) ([]string, error) {
	if !s.Enabled() {
		return nil, nil
	}
	if s.breaker != nil && !s.breaker.Allow() {
		s.logger.Warn("Postgres circuit open")
		return nil, sql.ErrConnDone
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT command FROM commands WHERE session_id = $1 ORDER BY executed_at ASC LIMIT $2
`, sessionID, limit)
	if err != nil {
		if s.breaker != nil {
			s.breaker.RecordFailure()
		}
		return nil, err
	}
	if s.breaker != nil {
		s.breaker.RecordSuccess()
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) InsertKernelEvent(ctx context.Context, sessionID, eventType string, pid, signal, exitCode int, raw string) error {
	if !s.Enabled() {
		return nil
	}
	_, err := s.exec(ctx, `
INSERT INTO kernel_events (session_id, event_type, pid, signal, exit_code, event_data)
VALUES ($1,$2,$3,$4,$5,$6::jsonb)
`, sessionID, eventType, pid, signal, exitCode, raw)
	return err
}

func (s *Store) InsertAIExplanation(ctx context.Context, sessionID, explanation, model string, tokens int) error {
	if !s.Enabled() {
		return nil
	}
	_, err := s.exec(ctx, `
INSERT INTO ai_explanations (session_id, explanation, model, tokens_used)
VALUES ($1,$2,$3,$4)
`, sessionID, explanation, model, tokens)
	return err
}

func (s *Store) GetSnapshot(ctx context.Context, sessionID string) (bucket, key string, err error) {
	if !s.Enabled() {
		return "", "", sql.ErrNoRows
	}
	if s.breaker != nil && !s.breaker.Allow() {
		s.logger.Warn("Postgres circuit open")
		return "", "", sql.ErrConnDone
	}
	err = s.DB.QueryRowContext(ctx, `
SELECT s3_bucket, s3_key FROM snapshots
WHERE session_id = $1
ORDER BY created_at DESC LIMIT 1
`, sessionID).Scan(&bucket, &key)
	if s.breaker != nil {
		if err != nil {
			s.breaker.RecordFailure()
		} else {
			s.breaker.RecordSuccess()
		}
	}
	return
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
