//go:build linux

package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

var containerIDPattern = regexp.MustCompile(`[a-f0-9]{64}`)

type sessionResolver struct {
	logger *zap.Logger
	client *http.Client
	mu     sync.Mutex
	cache  map[string]resolvedSession
}

type resolvedSession struct {
	sessionID string
	expiresAt time.Time
}

func newSessionResolver(logger *zap.Logger) *sessionResolver {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", "/var/run/docker.sock")
		},
	}
	return &sessionResolver{
		logger: logger,
		client: &http.Client{Transport: transport, Timeout: 2 * time.Second},
		cache:  map[string]resolvedSession{},
	}
}

func (r *sessionResolver) Resolve(pid int) string {
	procRoot := os.Getenv("HOST_PROC")
	if procRoot == "" {
		procRoot = "/proc"
	}
	data, err := os.ReadFile(procRoot + "/" + strconv.Itoa(pid) + "/cgroup")
	if err != nil {
		return ""
	}
	containerID := containerIDPattern.FindString(string(data))
	if containerID == "" {
		return ""
	}

	r.mu.Lock()
	if cached, ok := r.cache[containerID]; ok && time.Now().Before(cached.expiresAt) {
		r.mu.Unlock()
		return cached.sessionID
	}
	r.mu.Unlock()

	req, _ := http.NewRequest(http.MethodGet, "http://docker/containers/"+containerID+"/json", nil)
	resp, err := r.client.Do(req)
	if err != nil {
		r.logger.Debug("docker inspect for eBPF event failed", zap.Error(err))
		return ""
	}
	defer resp.Body.Close()
	var inspect struct {
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if json.NewDecoder(resp.Body).Decode(&inspect) != nil {
		return ""
	}
	sessionID := strings.TrimSpace(inspect.Config.Labels["entropy.session"])
	r.mu.Lock()
	r.cache[containerID] = resolvedSession{sessionID: sessionID, expiresAt: time.Now().Add(time.Minute)}
	r.mu.Unlock()
	return sessionID
}
