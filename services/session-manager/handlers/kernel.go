package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/entropy/pkg/db"
	"github.com/entropy/pkg/ebpf"
	"go.uber.org/zap"
)

type KernelHandler struct {
	store  *db.Store
	logger *zap.Logger
	ws     *WebSocketHandler
}

func NewKernelHandler(store *db.Store, logger *zap.Logger, ws *WebSocketHandler) *KernelHandler {
	return &KernelHandler{store: store, logger: logger, ws: ws}
}

func (h *KernelHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	var req ebpf.IngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for _, ev := range req.Events {
		raw, _ := json.Marshal(ev.Raw)
		_ = h.store.InsertKernelEvent(r.Context(), ev.SessionID, ev.EventType, ev.PID, ev.Signal, ev.ExitCode, string(raw))
		h.ws.NotifyKernelEvent(ev)
	}
	w.WriteHeader(http.StatusOK)
}
