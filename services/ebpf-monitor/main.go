package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/entropy/pkg/ebpf"
	"go.uber.org/zap"
)

type deduper struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func newDeduper() *deduper {
	return &deduper{seen: map[string]time.Time{}}
}

func (d *deduper) allow(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	if t, ok := d.seen[key]; ok && now.Sub(t) < 5*time.Second {
		return false
	}
	d.seen[key] = now
	return true
}

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	smURL := os.Getenv("SESSION_MANAGER_URL")
	if smURL == "" {
		smURL = "http://session-manager:8080"
	}
	simulator := os.Getenv("SIMULATOR_MODE") != "false"

	port := os.Getenv("PORT")
	if port == "" {
		port = "8083"
	}

	dedup := newDeduper()
	var bufMu sync.Mutex
	buffers := map[string][]ebpf.KernelEvent{}

	postEvents := func(events []ebpf.KernelEvent) {
		body, _ := json.Marshal(ebpf.IngestRequest{Events: events})
		req, _ := http.NewRequest(http.MethodPost, smURL+"/internal/kernel-events", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			logger.Warn("post kernel events failed", zap.Error(err))
			return
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= http.StatusMultipleChoices {
			logger.Warn("session manager rejected kernel events", zap.Int("status", resp.StatusCode))
		}
	}

	record := func(ev ebpf.KernelEvent) {
		if ev.SessionID == "" {
			logger.Debug("dropping unassigned kernel event",
				zap.String("event_type", ev.EventType),
				zap.Int("pid", ev.PID))
			return
		}
		key := ev.SessionID + ":" + ev.EventType
		if !dedup.allow(key) {
			return
		}
		bufMu.Lock()
		list := buffers[ev.SessionID]
		list = append(list, ev)
		if len(list) > 20 {
			list = list[len(list)-20:]
		}
		buffers[ev.SessionID] = list
		bufMu.Unlock()
		postEvents([]ebpf.KernelEvent{ev})
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		mode := "probes"
		if simulator {
			mode = "simulator"
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "mode": mode})
	})
	mux.HandleFunc("/internal/simulate", func(w http.ResponseWriter, r *http.Request) {
		if !simulator {
			http.Error(w, "simulator disabled", http.StatusNotFound)
			return
		}
		var ev ebpf.KernelEvent
		if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if ev.DetectedAt.IsZero() {
			ev.DetectedAt = time.Now().UTC()
		}
		record(ev)
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/internal/events", func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.URL.Query().Get("session_id")
		bufMu.Lock()
		events := append([]ebpf.KernelEvent(nil), buffers[sessionID]...)
		bufMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"events": events})
	})

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if !simulator {
		go func() {
			if err := runKernelSource(ctx, logger, record); err != nil {
				logger.Warn("eBPF probes unavailable; monitor remains healthy", zap.Error(err))
			}
		}()
	}

	logger.Info("eBPF monitor starting", zap.String("port", port), zap.Bool("simulator", simulator))
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
