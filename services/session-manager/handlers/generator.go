package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/entropy/pkg/airlock"
	"github.com/entropy/services/session-manager/labs"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	generatedLabsKey   = "labs:generated"
	generatorJobTTL    = 2 * time.Hour
	generatorPerHour   = 5
	generatorAttempts  = 2
	generatorMaxTokens = 3000
)

// GeneratorHandler turns a topic into a validated lab: the model writes a
// spec, static validation and a throwaway sandbox prove it is broken after
// setup and fixed by its reference fix, and only then is it published.
type GeneratorHandler struct {
	rdb     *redis.Client
	logger  *zap.Logger
	airlock *airlock.Client
	aiURL   string
	http    *http.Client
}

func NewGeneratorHandler(rdb *redis.Client, logger *zap.Logger) *GeneratorHandler {
	airURL := os.Getenv("AIRLOCK_URL")
	if airURL == "" {
		airURL = "http://airlock:8081"
	}
	aiURL := os.Getenv("AI_EXPLAINER_URL")
	if aiURL == "" {
		aiURL = "http://ai-explainer:8082"
	}
	return &GeneratorHandler{
		rdb: rdb, logger: logger, airlock: airlock.NewClient(airURL),
		aiURL: strings.TrimRight(aiURL, "/"), http: &http.Client{Timeout: 5 * time.Minute},
	}
}

// LoadPublished registers generated labs saved by earlier runs.
func (g *GeneratorHandler) LoadPublished(ctx context.Context) int {
	raw, err := g.rdb.HGetAll(ctx, generatedLabsKey).Result()
	if err != nil {
		return 0
	}
	n := 0
	for _, v := range raw {
		s, err := labs.ParseSpec([]byte(v))
		if err != nil || len(s.Validate()) > 0 {
			continue
		}
		l := s.Lab()
		l.Generated = true
		labs.Register(l)
		n++
	}
	return n
}

type generatorJob struct {
	ID       string       `json:"id"`
	State    string       `json:"state"` // generating, validating, published, failed
	Topic    string       `json:"topic"`
	Attempt  int          `json:"attempt"`
	Message  string       `json:"message,omitempty"`
	Problems []string     `json:"problems,omitempty"`
	LabID    string       `json:"labId,omitempty"`
	Title    string       `json:"title,omitempty"`
	Report   *labs.Report `json:"report,omitempty"`
	Updated  string       `json:"updated"`
}

func (g *GeneratorHandler) saveJob(ctx context.Context, j *generatorJob) {
	j.Updated = time.Now().UTC().Format(time.RFC3339)
	raw, _ := json.Marshal(j)
	_ = g.rdb.Set(ctx, "labgen:job:"+j.ID, raw, generatorJobTTL).Err()
}

// Generate handles POST (start a job: {"topic","difficulty"}) and GET
// (?job=<id> status).
func (g *GeneratorHandler) Generate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if r.Method == http.MethodGet {
		raw, err := g.rdb.Get(ctx, "labgen:job:"+r.URL.Query().Get("job")).Result()
		if err != nil {
			http.Error(w, "job not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(raw))
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "GET or POST", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Topic      string `json:"topic"`
		Difficulty string `json:"difficulty"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	req.Topic = strings.TrimSpace(req.Topic)
	if len(req.Topic) < 4 || len(req.Topic) > 200 {
		http.Error(w, "describe the incident in 4 to 200 characters", http.StatusBadRequest)
		return
	}
	switch req.Difficulty {
	case "Beginner", "Intermediate", "Advanced":
	default:
		req.Difficulty = "Intermediate"
	}
	owner := r.Header.Get("X-User-Id")
	if owner == "" {
		owner = "session:" + r.URL.Query().Get("session_id")
	}
	quotaKey := "labgen:quota:" + owner
	if n, _ := g.rdb.Incr(ctx, quotaKey).Result(); n == 1 {
		g.rdb.Expire(ctx, quotaKey, time.Hour)
	} else if n > generatorPerHour {
		http.Error(w, fmt.Sprintf("lab generation is limited to %d per hour", generatorPerHour), http.StatusTooManyRequests)
		return
	}

	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	job := &generatorJob{ID: hex.EncodeToString(buf), State: "generating", Topic: req.Topic, Attempt: 1}
	g.saveJob(ctx, job)
	go g.run(job, req.Topic, req.Difficulty)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(job)
}

func (g *GeneratorHandler) complete(ctx context.Context, prompt string) (string, error) {
	body, _ := json.Marshal(map[string]interface{}{"prompt": prompt, "maxTokens": generatorMaxTokens})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.aiURL+"/internal/complete-json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusServiceUnavailable {
		return "", fmt.Errorf("no AI model is configured for lab generation")
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return "", fmt.Errorf("generator error: %s", strings.TrimSpace(string(b)))
	}
	var out struct {
		JSON string `json:"json"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.JSON, nil
}

func (g *GeneratorHandler) stackAvailable(ctx context.Context) bool {
	r, err := g.newRunner(ctx)
	if err != nil {
		return false
	}
	defer r.Close()
	_, code, err := r.Run(ctx, `python3 -c 'import http.server'`)
	return err == nil && code == 0
}

func (g *GeneratorHandler) run(job *generatorJob, topic, difficulty string) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	stackOK := g.stackAvailable(ctx)
	var feedback []string
	for attempt := 1; attempt <= generatorAttempts; attempt++ {
		job.Attempt, job.State, job.Message = attempt, "generating", "Writing the lab."
		g.saveJob(ctx, job)
		raw, err := g.complete(ctx, labs.GeneratorPrompt(topic, difficulty, stackOK, feedback))
		if err != nil {
			job.State, job.Message = "failed", err.Error()
			g.saveJob(ctx, job)
			return
		}
		spec, err := labs.ParseSpec([]byte(labs.ExtractJSON(raw)))
		if err != nil {
			feedback = []string{err.Error()}
			continue
		}
		spec.ID = uniqueLabID(spec.ID)
		spec.Difficulty = difficulty
		if problems := spec.Validate(); len(problems) > 0 {
			feedback = problems
			job.Problems = problems
			continue
		}
		job.State, job.Message, job.Title = "validating", "Proving the lab in a throwaway sandbox.", spec.Title
		g.saveJob(ctx, job)
		settle := time.Duration(spec.SettleSeconds) * time.Second
		if settle == 0 {
			settle = 3 * time.Second
		}
		report := labs.ValidateInSandbox(ctx, g.newRunner, spec.Lab(), spec.ReferenceFix, settle)
		job.Report = &report
		if !report.OK {
			feedback = report.Problems
			job.Problems = report.Problems
			continue
		}
		raw2, _ := json.Marshal(spec)
		if err := g.rdb.HSet(ctx, generatedLabsKey, spec.ID, raw2).Err(); err != nil {
			job.State, job.Message = "failed", "validated, but could not be saved"
			g.saveJob(ctx, job)
			return
		}
		l := spec.Lab()
		l.Generated = true
		labs.Register(l)
		g.rdb.Incr(ctx, "metric:labs_generated")
		job.State, job.Message, job.LabID, job.Problems = "published", "Validated and published.", spec.ID, nil
		g.saveJob(ctx, job)
		g.logger.Info("generated lab published", zap.String("lab", spec.ID), zap.Int("attempt", attempt))
		return
	}
	job.State = "failed"
	job.Message = "The generated lab did not pass validation, so it was not published."
	job.Problems = feedback
	g.saveJob(ctx, job)
}

func uniqueLabID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if !strings.HasPrefix(id, "gen-") {
		id = "gen-" + id
	}
	if len(id) > 34 {
		id = id[:34]
	}
	if _, exists := labs.Get(id); exists {
		buf := make([]byte, 2)
		_, _ = rand.Read(buf)
		id = id + "-" + hex.EncodeToString(buf)
	}
	return strings.Trim(id, "-")
}

// airlockRunner validates labs in a throwaway sandbox spawned by Airlock.
type airlockRunner struct {
	client      *airlock.Client
	sessionID   string
	containerID string
}

func (g *GeneratorHandler) newRunner(ctx context.Context) (labs.Runner, error) {
	buf := make([]byte, 6)
	_, _ = rand.Read(buf)
	sessionID := "labgen-" + hex.EncodeToString(buf)
	id, err := g.airlock.Spawn(ctx, sessionID, "", "")
	if err != nil {
		return nil, err
	}
	return &airlockRunner{client: g.airlock, sessionID: sessionID, containerID: id}, nil
}

func (a *airlockRunner) Run(ctx context.Context, script string) (string, int, error) {
	var out strings.Builder
	code := -1
	err := a.client.ExecStream(ctx, airlock.ExecRequest{SessionID: a.sessionID, ContainerID: a.containerID, Command: script},
		func(ch airlock.ExecChunk) error {
			out.WriteString(ch.Stdout)
			out.WriteString(ch.Stderr)
			if ch.Completed {
				code = ch.ExitCode
			}
			return nil
		})
	return out.String(), code, err
}

func (a *airlockRunner) Close() {
	_ = a.client.Stop(context.Background(), airlock.StopRequest{SessionID: a.sessionID, ContainerID: a.containerID, GraceSec: 1})
}
