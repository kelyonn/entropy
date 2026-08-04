//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	pkgebpf "github.com/entropy/pkg/ebpf"
	"go.uber.org/zap"
)

type bpfKernelEvent struct {
	PID      uint32
	Value    int32
	Kind     uint32
	Reserved uint32
}

func runKernelSource(ctx context.Context, logger *zap.Logger, emit func(pkgebpf.KernelEvent)) error {
	if err := os.MkdirAll("/sys/fs/bpf", 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("prepare bpf filesystem: %w", err)
	}

	var objects entropybpfObjects
	if err := loadEntropybpfObjects(&objects, nil); err != nil {
		return fmt.Errorf("load bpf objects: %w", err)
	}
	defer objects.Close()

	killLink, err := link.Tracepoint("syscalls", "sys_enter_kill", objects.TraceSysEnterKill, nil)
	if err != nil {
		return fmt.Errorf("attach sys_enter_kill: %w", err)
	}
	defer killLink.Close()

	exitLink, err := link.Tracepoint("syscalls", "sys_enter_exit_group", objects.TraceSysExitGroup, nil)
	if err != nil {
		return fmt.Errorf("attach sys_enter_exit_group: %w", err)
	}
	defer exitLink.Close()

	oomLink, err := link.Kprobe("oom_kill_process", objects.TraceOomKillProcess, nil)
	if err != nil {
		return fmt.Errorf("attach oom_kill_process: %w", err)
	}
	defer oomLink.Close()

	panicLink, err := link.Kprobe("panic", objects.TraceKernelPanic, nil)
	if err != nil {
		logger.Warn("kernel panic probe unavailable", zap.Error(err))
	} else {
		defer panicLink.Close()
	}

	reader, err := perf.NewReader(objects.Events, os.Getpagesize()*8)
	if err != nil {
		return fmt.Errorf("open perf reader: %w", err)
	}
	defer reader.Close()

	go func() {
		<-ctx.Done()
		_ = reader.Close()
	}()

	resolver := newSessionResolver(logger)
	logger.Info("eBPF probes attached")
	for {
		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, perf.ErrClosed) || ctx.Err() != nil {
				return nil
			}
			logger.Warn("read perf event", zap.Error(err))
			continue
		}
		if record.LostSamples > 0 {
			logger.Warn("lost eBPF samples", zap.Uint64("count", record.LostSamples))
			continue
		}

		var raw bpfKernelEvent
		if err := binary.Read(bytes.NewReader(record.RawSample), binary.LittleEndian, &raw); err != nil {
			logger.Warn("decode eBPF event", zap.Error(err))
			continue
		}
		sessionID := resolver.Resolve(int(raw.PID))
		event := normalizeBPFEvent(raw, sessionID)
		emit(event)
	}
}

func normalizeBPFEvent(raw bpfKernelEvent, sessionID string) pkgebpf.KernelEvent {
	event := pkgebpf.KernelEvent{
		SessionID:  sessionID,
		PID:        int(raw.PID),
		DetectedAt: time.Now().UTC(),
		Raw: map[string]interface{}{
			"kind":  raw.Kind,
			"value": raw.Value,
		},
	}
	switch raw.Kind {
	case 1:
		event.EventType = pkgebpf.EventProcessKill
		event.Signal = int(raw.Value)
		event.Message = fmt.Sprintf("process %d received signal %d", raw.PID, raw.Value)
	case 2:
		event.ExitCode = int(raw.Value)
		if raw.Value == 139 {
			event.EventType = pkgebpf.EventSegfault
			event.Message = fmt.Sprintf("process %d exited after SIGSEGV", raw.PID)
		} else {
			event.EventType = "process_exit"
			event.Message = fmt.Sprintf("process %d exited with code %d", raw.PID, raw.Value)
		}
	case 3:
		event.EventType = pkgebpf.EventOOMKill
		event.ExitCode = 137
		event.Message = fmt.Sprintf("OOM killer selected process %d", raw.PID)
	case 4:
		event.EventType = pkgebpf.EventKernelPanic
		event.ExitCode = 99
		event.Message = "kernel panic detected"
	}
	return event
}
