package labs

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

// Completion records a finished lab for one learner.
type Completion struct {
	LabID       string `json:"labId"`
	CompletedAt string `json:"completedAt"`
	Seconds     int    `json:"seconds"`
	HintsUsed   int    `json:"hintsUsed"`
}

// Progress stores completions in Redis keyed by owner. Signed-in users own
// their progress across sessions; guests own it per session.
type Progress struct {
	rdb *redis.Client
}

func NewProgress(rdb *redis.Client) *Progress {
	return &Progress{rdb: rdb}
}

const progressTTL = 180 * 24 * time.Hour

func OwnerKey(userID, sessionID string) string {
	if userID != "" {
		return "labprogress:user:" + userID
	}
	return "labprogress:session:" + sessionID
}

// Record keeps the best run (fewest hints, then fastest) for each lab.
func (p *Progress) Record(ctx context.Context, owner string, c Completion) error {
	if prev, err := p.rdb.HGet(ctx, owner, c.LabID).Result(); err == nil {
		var old Completion
		if json.Unmarshal([]byte(prev), &old) == nil && better(old, c) {
			return nil
		}
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if err := p.rdb.HSet(ctx, owner, c.LabID, raw).Err(); err != nil {
		return err
	}
	return p.rdb.Expire(ctx, owner, progressTTL).Err()
}

func better(old, next Completion) bool {
	if old.HintsUsed != next.HintsUsed {
		return old.HintsUsed < next.HintsUsed
	}
	return old.Seconds <= next.Seconds
}

func (p *Progress) List(ctx context.Context, owner string) (map[string]Completion, error) {
	raw, err := p.rdb.HGetAll(ctx, owner).Result()
	if err != nil {
		return nil, err
	}
	out := make(map[string]Completion, len(raw))
	for id, v := range raw {
		var c Completion
		if json.Unmarshal([]byte(v), &c) == nil {
			out[id] = c
		}
	}
	return out, nil
}
