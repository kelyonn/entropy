package container

import "testing"

func TestNormalizeSignal(t *testing.T) {
	for in, want := range map[string]string{"SIGINT": "INT", "int": "INT", "": "INT", "SIGKILL": "KILL", " term ": "TERM"} {
		got, ok := NormalizeSignal(in)
		if !ok || got != want {
			t.Fatalf("NormalizeSignal(%q) = %q, %v", in, got, ok)
		}
	}
	for _, bad := range []string{"SIGSTOP", "9; rm -rf /", "USR1"} {
		if _, ok := NormalizeSignal(bad); ok {
			t.Fatalf("NormalizeSignal(%q) accepted", bad)
		}
	}
}

func TestComputeStats(t *testing.T) {
	var s containerStats
	s.CPUStats.CPUUsage.TotalUsage = 2_000
	s.PreCPUStats.CPUUsage.TotalUsage = 1_000
	s.CPUStats.SystemUsage = 20_000
	s.PreCPUStats.SystemUsage = 10_000
	s.CPUStats.OnlineCPUs = 4
	s.MemoryStats.Usage = 200
	s.MemoryStats.Limit = 400
	s.MemoryStats.Stats = map[string]uint64{"inactive_file": 100}
	s.PidsStats.Current = 42
	got := computeStats(s)
	if got.CPUPct != 40 {
		t.Fatalf("cpu = %v, want 40", got.CPUPct)
	}
	if got.MemPct != 25 || got.MemBytes != 100 {
		t.Fatalf("mem = %v (%d bytes), want 25 (100 bytes)", got.MemPct, got.MemBytes)
	}
	if got.Pids != 42 {
		t.Fatalf("pids = %d", got.Pids)
	}
}
