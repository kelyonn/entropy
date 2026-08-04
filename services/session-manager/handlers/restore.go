package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/entropy/pkg/airlock"
	"github.com/entropy/pkg/db"
	"github.com/entropy/services/session-manager/session"
	"github.com/entropy/services/session-manager/storage"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type RestoreHandler struct {
	rdb     *redis.Client
	logger  *zap.Logger
	store   *db.Store
	storage *storage.Client
	airlock *airlock.Client
	manager *session.Manager
}

func NewRestoreHandler(rdb *redis.Client, logger *zap.Logger, store *db.Store, storageClient *storage.Client, manager *session.Manager) *RestoreHandler {
	airURL := os.Getenv("AIRLOCK_URL")
	if airURL == "" {
		airURL = "http://airlock:8081"
	}
	return &RestoreHandler{
		rdb: rdb, logger: logger, store: store, storage: storageClient,
		airlock: airlock.NewClient(airURL), manager: manager,
	}
}

func (h *RestoreHandler) Snapshot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "session_id required", http.StatusBadRequest)
		return
	}

	raw, err := h.rdb.Get(ctx, "session:"+sessionID).Result()
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	var sess struct {
		ContainerID string `json:"container_id"`
	}
	_ = json.Unmarshal([]byte(raw), &sess)
	if sess.ContainerID == "" {
		http.Error(w, "no container for session", http.StatusBadRequest)
		return
	}

	tag := fmt.Sprintf("entropy-snapshot:%s", sessionID[:8])
	if _, err := h.airlock.Commit(ctx, sess.ContainerID, tag); err != nil {
		http.Error(w, "commit failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	gzData, err := h.airlock.SaveImageGzip(ctx, tag)
	if err != nil {
		http.Error(w, "save failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	key := fmt.Sprintf("snapshots/%s-%d.tar.gz", sessionID, time.Now().Unix())
	backend, err := h.storage.PutBestEffortWithRetry(ctx, key, gzData, 2)
	if err != nil {
		http.Error(w, "upload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	history, _ := h.store.ListCommands(ctx, sessionID, 50)
	if len(history) == 0 {
		history, _ = h.rdb.LRange(ctx, fmt.Sprintf("history:%s", sessionID), 0, 49).Result()
		reverseStrings(history)
	}
	histJSON, _ := json.Marshal(history)

	snapID := uuid.New().String()
	_ = h.store.InsertSnapshot(ctx, snapID, sessionID, h.storage.Bucket(), key, int64(len(gzData)), time.Now().Add(30*24*time.Hour))
	h.rdb.HSet(ctx, "snapshot:meta:"+sessionID,
		"key", key, "backend", backend, "id", snapID, "image", tag, "history", string(histJSON))
	h.rdb.Incr(ctx, "metric:snapshot_saves")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "saved", "snapshot_id": snapID, "backend": backend, "key": key,
	})
}

func (h *RestoreHandler) Restore(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "session_id required", http.StatusBadRequest)
		return
	}

	key, _ := h.rdb.HGet(ctx, "snapshot:meta:"+sessionID, "key").Result()
	imageTag, _ := h.rdb.HGet(ctx, "snapshot:meta:"+sessionID, "image").Result()
	if key == "" {
		if _, s3key, err := h.store.GetSnapshot(ctx, sessionID); err == nil {
			key = s3key
		}
	}
	if key == "" {
		http.Error(w, "snapshot not found", http.StatusNotFound)
		return
	}
	if imageTag == "" {
		imageTag = fmt.Sprintf("entropy-snapshot:%s", sessionID[:8])
	}

	data, err := h.storage.GetBestEffort(ctx, key)
	if err != nil {
		http.Error(w, "snapshot download failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	if len(data) == 0 {
		http.Error(w, "snapshot is empty", http.StatusBadGateway)
		return
	}
	if err := h.airlock.LoadImageGzip(ctx, data); err != nil {
		http.Error(w, "snapshot load failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	newID := uuid.New().String()
	containerID, err := h.airlock.Spawn(ctx, newID, "", imageTag)
	if err != nil {
		http.Error(w, "failed to spawn container: "+err.Error(), http.StatusInternalServerError)
		return
	}
	userID := r.Header.Get("X-User-Id")
	if err := h.manager.RegisterRestored(ctx, newID, userID, containerID); err != nil {
		_ = h.airlock.Stop(ctx, airlock.StopRequest{SessionID: newID, ContainerID: containerID, GraceSec: 5})
		http.Error(w, "failed to register restored session", http.StatusInternalServerError)
		return
	}
	_ = h.store.UpsertSession(ctx, newID, userID, containerID, string(session.StatusActive))

	history, _ := h.store.ListCommands(ctx, sessionID, 50)
	if len(history) == 0 {
		if historyJSON, ok := h.rdb.HGet(ctx, "snapshot:meta:"+sessionID, "history").Result(); ok == nil {
			_ = json.Unmarshal([]byte(historyJSON), &history)
		}
	}
	histKey := fmt.Sprintf("history:%s", newID)
	for i := len(history) - 1; i >= 0; i-- {
		h.rdb.LPush(ctx, histKey, history[i])
	}
	h.rdb.HSet(ctx, "restore:replay:"+newID, "commands", fmt.Sprintf("%d", len(history)))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "restored", "session_id": newID,
		"container_id": containerID, "ws_url": "/ws?session_id=" + newID,
		"history_replay": fmt.Sprintf("%d commands", len(history)),
	})
}

func reverseStrings(values []string) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}
