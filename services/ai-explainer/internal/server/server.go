package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	pkgai "github.com/entropy/pkg/ai"
	"github.com/entropy/services/ai-explainer/ai"
	"github.com/entropy/services/ai-explainer/middleware"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type Server struct {
	provider ai.ExplainerProvider
	limiter *middleware.RateLimiter
	logger  *zap.Logger
}

func New(rdb *redis.Client, logger *zap.Logger) *Server {
	ollamaURL := os.Getenv("OLLAMA_URL")
	ollamaModel := os.Getenv("OLLAMA_MODEL")
	return &Server{
		provider: ai.NewOllamaClient(rdb, logger, ollamaURL, ollamaModel),
		limiter: middleware.NewRateLimiter(rdb),
		logger:  logger,
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/internal/explain", s.handleExplain)
	return mux
}

func (s *Server) emitToken(w http.ResponseWriter, flusher http.Flusher, token string, done, fallback bool) {
	b, _ := json.Marshal(pkgai.ExplainToken{Token: token, Done: done, Fallback: fallback})
	fmt.Fprintf(w, "data: %s\n\n", b)
	if flusher != nil {
		flusher.Flush()
	}
}

func (s *Server) handleExplain(w http.ResponseWriter, r *http.Request) {
	var req pkgai.ExplainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !s.limiter.Allow(r.Context(), req.SessionID) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		s.emitToken(w, flusher, "Rate limit reached. Try again in 1 hour.", false, false)
		s.emitToken(w, flusher, "", true, false)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)

	emit := func(token string, done bool, fallback bool) {
		s.emitToken(w, flusher, token, done, fallback)
	}

	streamFn := s.provider.ExplainCrashStream
	if req.Simpler {
		streamFn = s.provider.ExplainSimplerStream
	} else if req.Question != "" {
		_ = s.provider.ExplainQuestionStream(r.Context(), req.Command, req.Stderr, req.ExitCode, req.Question, emit)
		return
	}

	_ = streamFn(r.Context(), req.Command, req.Stderr, req.ExitCode, emit)
}
