package server

import (
	"encoding/json"
	"net/http"

	"github.com/entropy/pkg/airlock"
	"github.com/entropy/services/airlock/container"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type Server struct {
	orch   *container.Orchestrator
	rdb    *redis.Client
	logger *zap.Logger
}

func New(rdb *redis.Client, logger *zap.Logger) *Server {
	orch, err := container.NewOrchestrator(rdb, logger)
	if err != nil {
		logger.Fatal("docker client", zap.Error(err))
	}
	return &Server{
		orch:   orch,
		rdb:    rdb,
		logger: logger,
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/internal/spawn", s.handleSpawn)
	mux.HandleFunc("/internal/exec", s.handleExec)
	mux.HandleFunc("/internal/stats", s.handleStats)
	mux.HandleFunc("/internal/kill", s.handleKill)
	mux.HandleFunc("/internal/stop", s.handleStop)
	mux.HandleFunc("/internal/commit", s.handleCommit)
	mux.HandleFunc("/internal/save-image", s.handleSaveImage)
	mux.HandleFunc("/internal/load-image", s.handleLoadImage)
	return mux
}

func (s *Server) handleSpawn(w http.ResponseWriter, r *http.Request) {
	var req airlock.SpawnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id, err := s.orch.EnsureContainer(r.Context(), req.SessionID, req.ExistingContainerID, req.Image)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(airlock.SpawnResponse{ContainerID: id})
}

func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	var req airlock.ExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	flusher, _ := w.(http.Flusher)
	exec, err := container.NewExecutor(req.ContainerID, s.logger)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ch := make(chan container.StreamChunk, 32)
	errCh := make(chan error, 1)
	go func() {
		errCh <- exec.RunStreaming(r.Context(), req.Command, ch)
		close(ch)
	}()
	enc := json.NewEncoder(w)
	for c := range ch {
		chunk := airlock.ExecChunk{
			Stdout: c.Stdout, Stderr: c.Stderr,
			Completed: c.Completed, ExitCode: c.ExitCode,
		}
		_ = enc.Encode(chunk)
		if flusher != nil {
			flusher.Flush()
		}
	}
	if err := <-errCh; err != nil {
		s.logger.Debug("exec ended", zap.Error(err))
	}
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	var req airlock.StatsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cpu, mem, err := s.orch.StatsCPUMem(r.Context(), req.ContainerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(airlock.StatsResponse{CPU: int(cpu), Memory: int(mem)})
}

func (s *Server) handleKill(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	var req airlock.StopRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	grace := req.GraceSec
	if grace <= 0 {
		grace = 5
	}
	if err := s.orch.StopGraceful(r.Context(), req.ContainerID, grace); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleCommit(w http.ResponseWriter, r *http.Request) {
	var req airlock.CommitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tag, err := s.orch.Commit(r.Context(), req.ContainerID, req.Tag)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(airlock.CommitResponse{Tag: tag})
}

func (s *Server) handleSaveImage(w http.ResponseWriter, r *http.Request) {
	var req airlock.SaveImageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	data, err := s.orch.SaveImageGzip(r.Context(), req.Tag)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	_, _ = w.Write(data)
}

func (s *Server) handleLoadImage(w http.ResponseWriter, r *http.Request) {
	var req airlock.LoadImageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.orch.LoadImageGzip(r.Context(), req.Data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
