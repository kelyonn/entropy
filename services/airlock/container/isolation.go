package container

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/docker/docker/api/types/container"
	units "github.com/docker/go-units"
	"go.uber.org/zap"
)

// Isolation describes how sandboxes are confined. Entropy runs arbitrary root
// commands from anonymous users, so a shared-kernel runc container is not a
// sufficient boundary on a public host. gVisor (runsc) interposes a user-space
// kernel; it is preferred whenever the Docker daemon has it registered.
type Isolation struct {
	Runtime string
	// Strict refuses to start sandboxes when the requested runtime is missing.
	Strict bool
}

// sandboxCaps is the minimum set the labs need: CHOWN for chgrp/chown in the
// permissions labs, SETUID/SETGID for runuser. DAC_OVERRIDE is not granted,
// so root inside the sandbox still honors mode bits on other users' files.
var sandboxCaps = []string{"CHOWN", "SETUID", "SETGID"}

type runtimeLister interface {
	Runtimes(ctx context.Context) (map[string]bool, error)
}

// ResolveIsolation decides which OCI runtime to use.
//
//	SANDBOX_RUNTIME=auto (default)  use runsc when registered, else runc
//	SANDBOX_RUNTIME=runsc           require runsc; fail closed if missing
//	SANDBOX_RUNTIME=runc            shared kernel; local development only
func ResolveIsolation(ctx context.Context, l runtimeLister, logger *zap.Logger) (Isolation, error) {
	want := strings.TrimSpace(strings.ToLower(os.Getenv("SANDBOX_RUNTIME")))
	if want == "" {
		want = "auto"
	}
	available, err := l.Runtimes(ctx)
	if err != nil {
		available = map[string]bool{}
	}
	switch want {
	case "auto":
		if available["runsc"] {
			logger.Info("sandbox isolation: gVisor (runsc)")
			return Isolation{Runtime: "runsc"}, nil
		}
		logger.Warn("sandbox isolation: runc (shared host kernel). Install gVisor before exposing Entropy publicly; see docs/isolation.md")
		return Isolation{Runtime: ""}, nil
	case "runc":
		logger.Warn("sandbox isolation: runc forced by SANDBOX_RUNTIME; do not expose publicly")
		return Isolation{Runtime: ""}, nil
	default:
		if !available[want] {
			return Isolation{}, fmt.Errorf("SANDBOX_RUNTIME=%s is not registered with the Docker daemon", want)
		}
		logger.Info("sandbox isolation", zap.String("runtime", want))
		return Isolation{Runtime: want, Strict: true}, nil
	}
}

// apply sets the runtime and confinement options on a sandbox host config.
// Docker's default masked and read-only /proc paths stay in effect because
// MaskedPaths and ReadonlyPaths are left unset.
func (i Isolation) apply(hc *container.HostConfig) {
	hc.Runtime = i.Runtime
	hc.CapDrop = []string{"ALL"}
	hc.CapAdd = append([]string(nil), sandboxCaps...)
	hc.SecurityOpt = []string{"no-new-privileges:true"}
	hc.Ulimits = []*units.Ulimit{
		{Name: "nofile", Soft: 1024, Hard: 2048},
		{Name: "fsize", Soft: 512 << 20, Hard: 512 << 20},
	}
}
