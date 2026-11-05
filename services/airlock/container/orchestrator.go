package container

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type Orchestrator struct {
	isolation Isolation
	docker    *client.Client
	rdb       *redis.Client
	logger    *zap.Logger
}

func NewOrchestrator(rdb *redis.Client, logger *zap.Logger) (*Orchestrator, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	o := &Orchestrator{docker: cli, rdb: rdb, logger: logger}
	iso, err := ResolveIsolation(context.Background(), dockerRuntimes{cli}, logger)
	if err != nil {
		return nil, err
	}
	o.isolation = iso
	return o, nil
}

// Isolation reports the active sandbox runtime for health and metrics.
func (o *Orchestrator) Isolation() Isolation { return o.isolation }

type dockerRuntimes struct{ cli *client.Client }

func (d dockerRuntimes) Runtimes(ctx context.Context) (map[string]bool, error) {
	info, err := d.cli.Info(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(info.Runtimes))
	for name := range info.Runtimes {
		out[name] = true
	}
	return out, nil
}

func (o *Orchestrator) Client() *client.Client {
	return o.docker
}

func (o *Orchestrator) IsRunning(ctx context.Context, containerID string) bool {
	if containerID == "" {
		return false
	}
	inspect, err := o.docker.ContainerInspect(ctx, containerID)
	if err != nil {
		return false
	}
	return inspect.State != nil && inspect.State.Running
}

func (o *Orchestrator) EnsureContainer(ctx context.Context, sessionID, existingID, image string) (string, error) {
	if existingID != "" && o.IsRunning(ctx, existingID) {
		o.logger.Info("Reusing container",
			zap.String("container_id", existingID),
			zap.String("session_id", sessionID))
		return existingID, nil
	}
	return o.SpawnContainerWithRetry(ctx, sessionID, image)
}

func (o *Orchestrator) SpawnContainerWithRetry(ctx context.Context, sessionID, image string) (string, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<attempt) * time.Second
			o.logger.Warn("retrying container spawn",
				zap.Int("attempt", attempt+1),
				zap.Duration("delay", delay))
			time.Sleep(delay)
		}
		id, err := o.spawnContainerOnce(ctx, sessionID, image)
		if err == nil {
			return id, nil
		}
		lastErr = err
	}
	return "", fmt.Errorf("spawn failed after 3 attempts: %w", lastErr)
}

// fallbackImage is the stock userland every lab is written against.
const fallbackImage = "ubuntu:22.04"

// sandboxImage prefers the tooled entropy-sandbox image (procps, lsof,
// iproute2, strace) and falls back to stock Ubuntu when it is not built.
func (o *Orchestrator) sandboxImage(ctx context.Context, requested string) string {
	if requested != "" {
		return requested
	}
	preferred := strings.TrimSpace(os.Getenv("SANDBOX_IMAGE"))
	if preferred == "" || preferred == fallbackImage {
		return fallbackImage
	}
	if _, _, err := o.docker.ImageInspectWithRaw(ctx, preferred); err != nil {
		o.logger.Warn("sandbox image not available, using fallback",
			zap.String("image", preferred), zap.String("fallback", fallbackImage))
		return fallbackImage
	}
	return preferred
}

func (o *Orchestrator) spawnContainerOnce(ctx context.Context, sessionID, image string) (string, error) {
	image = o.sandboxImage(ctx, image)
	pids := int64(100)
	hostConfig := &container.HostConfig{
		Resources: container.Resources{
			Memory:     256 * 1024 * 1024,
			MemorySwap: 256 * 1024 * 1024,
			NanoCPUs:   500000000,
			PidsLimit:  &pids,
		},
		NetworkMode: "none",
		Tmpfs: map[string]string{
			"/tmp":     "rw,noexec,nosuid,size=256m",
			"/var/log": "rw,noexec,nosuid,size=64m",
		},
		AutoRemove: false,
	}
	o.isolation.apply(hostConfig)
	resp, err := o.docker.ContainerCreate(ctx,
		&container.Config{
			Image:      image,
			Cmd:        []string{"/bin/bash", "-c", "mkdir -p /sandbox && cd /sandbox && exec /bin/bash"},
			Hostname:   "entropy",
			Tty:        true,
			OpenStdin:  true,
			WorkingDir: "/sandbox",
			Env:        []string{"ENTROPY_SESSION=" + sessionID},
			Labels: map[string]string{
				"entropy.session": sessionID,
				"entropy.type":    "sandbox",
			},
		},
		hostConfig,
		nil, nil, "entropy-"+sessionID[:8],
	)
	if err != nil {
		return "", err
	}
	if err := o.docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", err
	}
	o.logger.Info("Container spawned",
		zap.String("container_id", resp.ID),
		zap.String("session_id", sessionID))
	return resp.ID, nil
}

func (o *Orchestrator) StopGraceful(ctx context.Context, containerID string, graceSec int) error {
	if containerID == "" {
		return nil
	}
	timeout := graceSec
	_ = o.docker.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
	return o.docker.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
}

func (o *Orchestrator) DeleteContainer(ctx context.Context, sessionID, containerID string) {
	if containerID == "" {
		return
	}
	o.saveMetadata(ctx, sessionID, containerID)
	job, _ := json.Marshal(map[string]string{
		"session_id": sessionID, "container_id": containerID,
		"timestamp": time.Now().Format(time.RFC3339),
	})
	o.rdb.LPush(ctx, "jobs:snapshots", job)
	_ = o.StopGraceful(ctx, containerID, 5)
	o.logger.Info("Container removed", zap.String("container_id", containerID))
}

func (o *Orchestrator) Commit(ctx context.Context, containerID, tag string) (string, error) {
	resp, err := o.docker.ContainerCommit(ctx, containerID, container.CommitOptions{Reference: tag})
	if err != nil {
		return "", err
	}
	_ = resp
	return tag, nil
}

func (o *Orchestrator) SaveImageGzip(ctx context.Context, tag string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", "save", tag)
	var rawTar bytes.Buffer
	cmd.Stdout = &rawTar
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	var gz bytes.Buffer
	gw := gzip.NewWriter(&gz)
	if _, err := gw.Write(rawTar.Bytes()); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return gz.Bytes(), nil
}

func (o *Orchestrator) LoadImageGzip(ctx context.Context, data []byte) error {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	var tar bytes.Buffer
	if _, err := io.Copy(&tar, gr); err != nil {
		_ = gr.Close()
		return err
	}
	_ = gr.Close()
	cmd := exec.CommandContext(ctx, "docker", "load")
	cmd.Stdin = &tar
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker load: %w: %s", err, string(out))
	}
	return nil
}

// Stats is one telemetry sample for a sandbox.
type Stats struct {
	CPUPct   float64
	MemPct   float64
	MemBytes uint64
	MemLimit uint64
	Pids     int
}

func (o *Orchestrator) StatsCPUMem(ctx context.Context, containerID string) (float64, float64, error) {
	st, err := o.Stats(ctx, containerID)
	return st.CPUPct, st.MemPct, err
}

func (o *Orchestrator) Stats(ctx context.Context, containerID string) (Stats, error) {
	stats, err := o.docker.ContainerStats(ctx, containerID, false)
	if err != nil {
		return Stats{}, err
	}
	defer stats.Body.Close()
	var s containerStats
	if err := json.NewDecoder(stats.Body).Decode(&s); err != nil {
		return Stats{}, err
	}
	return computeStats(s), nil
}

func computeStats(s containerStats) Stats {
	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage - s.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(s.CPUStats.SystemUsage - s.PreCPUStats.SystemUsage)
	cpuPct := 0.0
	if systemDelta > 0 && cpuDelta > 0 {
		cpus := float64(s.CPUStats.OnlineCPUs)
		if cpus == 0 {
			cpus = float64(len(s.CPUStats.CPUUsage.PercpuUsage))
		}
		if cpus == 0 {
			cpus = 1
		}
		cpuPct = (cpuDelta / systemDelta) * cpus * 100.0
	}
	// Report usage the way `docker stats` does: exclude reclaimable page cache.
	used := s.MemoryStats.Usage
	cache := s.MemoryStats.Stats["inactive_file"]
	if cache == 0 {
		cache = s.MemoryStats.Stats["total_inactive_file"]
	}
	if cache > 0 && cache < used {
		used -= cache
	}
	memPct := 0.0
	if s.MemoryStats.Limit > 0 {
		memPct = float64(used) / float64(s.MemoryStats.Limit) * 100.0
	}
	return Stats{
		CPUPct: cpuPct, MemPct: memPct,
		MemBytes: used, MemLimit: s.MemoryStats.Limit,
		Pids: int(s.PidsStats.Current),
	}
}

type containerStats struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage  uint64   `json:"total_usage"`
			PercpuUsage []uint64 `json:"percpu_usage"`
		} `json:"cpu_usage"`
		SystemUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs  uint32 `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64            `json:"usage"`
		Limit uint64            `json:"limit"`
		Stats map[string]uint64 `json:"stats"`
	} `json:"memory_stats"`
	PidsStats struct {
		Current uint64 `json:"current"`
	} `json:"pids_stats"`
}

func (o *Orchestrator) saveMetadata(ctx context.Context, sessionID, containerID string) {
	key := fmt.Sprintf("container:deleted:%s", sessionID)
	fileTree := o.captureFileTree(ctx, containerID)
	fileTreeJSON, _ := json.Marshal(fileTree)
	history, _ := o.rdb.LRange(ctx, fmt.Sprintf("history:%s", sessionID), 0, 99).Result()
	historyJSON, _ := json.Marshal(history)
	o.rdb.HSet(ctx, key,
		"container_id", containerID,
		"file_tree", string(fileTreeJSON),
		"command_history", string(historyJSON),
		"deleted_at", time.Now().Format(time.RFC3339),
	)
	o.rdb.Expire(ctx, key, 30*24*time.Hour)
}

func (o *Orchestrator) captureFileTree(ctx context.Context, containerID string) map[string]string {
	fileTree := map[string]string{}
	reader, _, err := o.docker.CopyFromContainer(ctx, containerID, "/sandbox")
	if err != nil {
		return fileTree
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		return fileTree
	}
	fileTree["/sandbox"] = string(content)
	return fileTree
}
