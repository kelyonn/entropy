package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	pkgai "github.com/entropy/pkg/ai"
	"github.com/entropy/services/ai-explainer/ai"
	"github.com/entropy/services/ai-explainer/middleware"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type Server struct {
	provider ai.ExplainerProvider
	limiter  *middleware.RateLimiter
	logger   *zap.Logger
}

func New(rdb *redis.Client, logger *zap.Logger) *Server {
	ollamaURL := os.Getenv("OLLAMA_URL")
	ollamaModel := os.Getenv("OLLAMA_MODEL")
	return &Server{
		provider: ai.NewOllamaClient(rdb, logger, ollamaURL, ollamaModel),
		limiter:  middleware.NewRateLimiter(rdb),
		logger:   logger,
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/internal/explain", s.handleExplain)
	mux.HandleFunc("/internal/complete-json", s.handleCompleteJSON)
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
		if req.Auto && req.Question == "" {
			s.emitToken(w, flusher, s.provider.FallbackForCommand(req.Command, req.ExitCode)+budgetNote, false, true)
		} else {
			s.emitToken(w, flusher, "Rate limit reached. Try again in 1 hour.", false, false)
		}
		s.emitToken(w, flusher, "", true, false)
		return
	}

	req.Stderr = composeOutput(req.Stderr, req.Context)

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

const budgetNote = "\n\n_This session has used its AI explanations for the hour, so this is a prepared explanation._"

// composeOutput joins the command output and optional learner context into
// the block the prompts call "Error output". The output keeps its tail (where
// errors usually are) and the context is clearly labeled so the model does
// not mistake it for program output.
func composeOutput(output, context string) string {
	const maxOutput = 1400
	const maxContext = 1700
	output = strings.TrimSpace(output)
	if len(output) > maxOutput {
		output = "..." + output[len(output)-maxOutput:]
	}
	context = strings.TrimSpace(context)
	if context == "" {
		return output
	}
	if len(context) > maxContext {
		context = context[:maxContext]
	}
	if output == "" {
		output = "(no output)"
	}
	return output + "\n\n--- Learner context (not program output) ---\n" + context
}

// handleCompleteJSON returns one JSON completion: {"prompt": "...",
// "maxTokens": 3000} -> {"json": "..."}. 503 when no model is available.
func (s *Server) handleCompleteJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Prompt    string `json:"prompt"`
		MaxTokens int    `json:"maxTokens"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Prompt) == "" {
		http.Error(w, "prompt required", http.StatusBadRequest)
		return
	}
	completer, ok := s.provider.(ai.JSONCompleter)
	if !ok {
		http.Error(w, "generator unavailable", http.StatusServiceUnavailable)
		return
	}
	out, err := completer.CompleteJSON(r.Context(), req.Prompt, req.MaxTokens)
	if err != nil {
		status := http.StatusBadGateway
		if err == ai.ErrGeneratorUnavailable {
			status = http.StatusServiceUnavailable
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"json": out})
}
