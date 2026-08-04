package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/entropy/pkg/db"
	"github.com/entropy/services/session-manager/session"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type SessionHandler struct {
	rdb     *redis.Client
	logger  *zap.Logger
	manager *session.Manager
	store   *db.Store
}

func NewSessionHandler(rdb *redis.Client, logger *zap.Logger, manager *session.Manager, store *db.Store) *SessionHandler {
	return &SessionHandler{rdb: rdb, logger: logger, manager: manager, store: store}
}

func (h *SessionHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Header.Get("X-User-Id")
	queueToken := r.URL.Query().Get("queue_token")

	s, queue, err := h.manager.Create(ctx, userID, queueToken)
	if err != nil {
		h.rdb.Incr(ctx, "metric:session_creation_failures")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if queue != nil && queue.Queued {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(queue)
		return
	}

	_ = h.store.UpsertSession(ctx, s.ID, userID, "", string(session.StatusCreating))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"session_id": s.ID,
		"status":     string(s.Status),
		"ws_url":     "/ws?session_id=" + s.ID,
	})
}

func (h *SessionHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	keys := []string{
		"metric:total_sessions",
		"metric:total_commands",
		"metric:total_crashes",
		"metric:ai_explanations",
		"metric:queue_depth",
		"metric:session_summaries",
	}
	result := map[string]string{}
	for _, key := range keys {
		val, _ := h.rdb.Get(ctx, key).Result()
		if val == "" {
			val = "0"
		}
		result[key] = val
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *SessionHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"sessions": []interface{}{}})
		return
	}
	sessions, err := h.manager.ListForUser(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type item struct {
		ID        string `json:"id"`
		CreatedAt string `json:"created_at"`
		Status    string `json:"status"`
	}
	out := make([]item, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, item{
			ID:        s.ID,
			CreatedAt: s.CreatedAt.Format("2006-01-02 15:04"),
			Status:    string(s.Status),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"sessions": out})
}
