package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/entropy/services/session-manager/labs"
	"github.com/entropy/services/session-manager/session"
	"github.com/redis/go-redis/v9"
)

// LabsHandler serves the lab catalog and the caller's progress.
type LabsHandler struct {
	manager  *session.Manager
	progress *labs.Progress
}

func NewLabsHandler(rdb *redis.Client, manager *session.Manager) *LabsHandler {
	return &LabsHandler{manager: manager, progress: labs.NewProgress(rdb)}
}

func (h *LabsHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.Header.Get("X-User-Id")
	sessionID := r.URL.Query().Get("session_id")
	if userID == "" && sessionID != "" {
		if s, err := h.manager.Get(ctx, sessionID); err == nil {
			userID = s.UserID
		}
	}
	progress := map[string]labs.Completion{}
	if userID != "" || sessionID != "" {
		if p, err := h.progress.List(ctx, labs.OwnerKey(userID, sessionID)); err == nil {
			progress = p
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"labs":     labs.All(),
		"progress": progress,
	})
}
