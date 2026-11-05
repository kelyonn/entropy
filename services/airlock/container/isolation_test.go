package container

import (
	"context"
	"errors"
	"testing"

	"github.com/docker/docker/api/types/container"
	"go.uber.org/zap"
)

type fakeRuntimes struct {
	m   map[string]bool
	err error
}

func (f fakeRuntimes) Runtimes(context.Context) (map[string]bool, error) { return f.m, f.err }

func TestResolveIsolation(t *testing.T) {
	log := zap.NewNop()
	cases := []struct {
		env     string
		have    map[string]bool
		err     error
		want    string
		wantErr bool
	}{
		{"", map[string]bool{"runc": true, "runsc": true}, nil, "runsc", false},
		{"auto", map[string]bool{"runc": true}, nil, "", false},
		{"", nil, errors.New("daemon down"), "", false},
		{"runc", map[string]bool{"runsc": true}, nil, "", false},
		{"runsc", map[string]bool{"runc": true}, nil, "", true},
		{"kata", map[string]bool{"kata": true}, nil, "kata", false},
	}
	for _, c := range cases {
		t.Setenv("SANDBOX_RUNTIME", c.env)
		iso, err := ResolveIsolation(context.Background(), fakeRuntimes{c.have, c.err}, log)
		if (err != nil) != c.wantErr {
			t.Fatalf("env=%q have=%v: err=%v wantErr=%v", c.env, c.have, err, c.wantErr)
		}
		if err == nil && iso.Runtime != c.want {
			t.Fatalf("env=%q have=%v: runtime=%q want %q", c.env, c.have, iso.Runtime, c.want)
		}
	}
}

func TestIsolationApply(t *testing.T) {
	hc := &container.HostConfig{}
	Isolation{Runtime: "runsc"}.apply(hc)
	if hc.Runtime != "runsc" || len(hc.CapDrop) != 1 || hc.CapDrop[0] != "ALL" {
		t.Fatalf("unexpected host config %+v", hc)
	}
	for _, c := range hc.CapAdd {
		if c == "DAC_OVERRIDE" || c == "SYS_ADMIN" || c == "NET_ADMIN" {
			t.Fatalf("dangerous capability %s granted", c)
		}
	}
	if hc.ReadonlyPaths != nil || hc.MaskedPaths != nil {
		t.Fatal("default masked paths must not be overridden")
	}
}
