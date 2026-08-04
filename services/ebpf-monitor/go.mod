module github.com/entropy/services/ebpf-monitor

go 1.26

require (
	github.com/cilium/ebpf v0.22.0
	github.com/entropy/pkg v0.0.0
	go.uber.org/zap v1.27.0
)

require (
	go.uber.org/multierr v1.10.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
)

replace github.com/entropy/pkg => ../../pkg
