//go:build !ebpf

package ebpf

import (
    "errors"

    "go.uber.org/zap"
)

var errEBPFDisabled = errors.New("ebpf tracer disabled: build with -tags ebpf")

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
    if t.logger != nil {
        t.logger.Warn(errEBPFDisabled.Error())
    }
    return errEBPFDisabled
}

func (t *Tracer) Events() <-chan CrashEvent {
    return t.events
}
