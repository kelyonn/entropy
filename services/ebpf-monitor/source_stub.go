//go:build !linux

package main

import (
	"context"
	"fmt"

	pkgebpf "github.com/entropy/pkg/ebpf"
	"go.uber.org/zap"
)

func runKernelSource(_ context.Context, _ *zap.Logger, _ func(pkgebpf.KernelEvent)) error {
	return fmt.Errorf("real eBPF probes require Linux; set SIMULATOR_MODE=true")
}
