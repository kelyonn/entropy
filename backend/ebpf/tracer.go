//go:build ebpf

package ebpf

import (
    "bytes"
    _ "embed"
    "encoding/binary"

    "github.com/cilium/ebpf/link"
    "github.com/cilium/ebpf/ringbuf"
    "go.uber.org/zap"
)

//go:embed bpf/tracer.o
var tracerBPF []byte

type CrashEvent struct {
    PID      uint32
    ExitCode int32
    Comm     [16]byte
}

type Tracer struct {
    logger *zap.Logger
    events chan CrashEvent
}

func NewTracer(logger *zap.Logger) *Tracer {
    return &Tracer{
        logger: logger,
        events: make(chan CrashEvent, 100),
    }
}

func (t *Tracer) Start() error {
    // Load pre-compiled eBPF object
    spec, err := loadSpec()
    if err != nil {
        return err
    }

    objs := &tracerObjects{}
    if err := spec.LoadAndAssign(objs, nil); err != nil {
        return err
    }

    // Attach to sys_exit_group tracepoint
    tp, err := link.Tracepoint("syscalls", "sys_exit_group", objs.TraceExit, nil)
    if err != nil {
        return err
    }
    defer tp.Close()

    // Read events from ring buffer
    rd, err := ringbuf.NewReader(objs.Events)
    if err != nil {
        return err
    }
    defer rd.Close()

    go func() {
        for {
            record, err := rd.Read()
            if err != nil {
                break
            }
            var event CrashEvent
            binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event)
            t.events <- event
            t.logger.Info("Crash event captured",
                zap.Uint32("pid", event.PID),
                zap.Int32("exit_code", event.ExitCode))
        }
    }()

    return nil
}

func (t *Tracer) Events() <-chan CrashEvent {
    return t.events
}
