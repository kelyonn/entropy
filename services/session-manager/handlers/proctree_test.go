package handlers

import (
	"testing"
	"time"
)

func stat(pid, ppid int, comm, state string, utime int) string {
	// pid (comm) state ppid pgrp session tty tpgid flags minflt cminflt majflt cmajflt utime stime ...
	return itoa(pid) + " (" + comm + ") " + state + " " + itoa(ppid) + " 1 1 0 -1 0 0 0 0 0 " + itoa(utime) + " 0 0 0 20 0 1 0"
}

func itoa(i int) string {
	return func() string {
		s := ""
		if i == 0 {
			return "0"
		}
		for i > 0 {
			s = string(rune('0'+i%10)) + s
			i /= 10
		}
		return s
	}()
}

func TestParseProcLine(t *testing.T) {
	p, ok := parseProcLine(stat(47, 1, "reaper-svc --wo", "Z", 5) + "\t250\treaper-svc --workers 12")
	if !ok || p.PID != 47 || p.PPID != 1 || p.State != "Z" || p.Comm != "reaper-svc --wo" || p.RSSKB != 1000 || p.ticks != 5 {
		t.Fatalf("parse = %+v %v", p, ok)
	}
	k, _ := parseProcLine(stat(3, 0, "kthread", "S", 0) + "\t0\t")
	if k.Args != "[kthread]" {
		t.Fatalf("empty cmdline args = %q", k.Args)
	}
	if _, ok := parseProcLine("garbage"); ok {
		t.Fatal("garbage parsed")
	}
}

func TestBuildProcTreeHidesPlumbingAndComputesCPU(t *testing.T) {
	out := stat(1, 0, "bash", "S", 0) + "\t100\t/bin/bash\n" +
		stat(10, 0, "bash", "S", 0) + "\t100\t/bin/bash -c set -m; \"$@\" & entropy-command /bin/bash -c ls\n" +
		stat(11, 10, "bash", "S", 0) + "\t100\t/bin/bash -c . /root/.entropy/rc\n" +
		stat(12, 11, "perl", "R", 150) + "\t100\tperl -e 1 while 1\n" +
		stat(20, 0, "bash", "S", 0) + "\t100\t/bin/bash -c # ENTROPY_PROCTREE\n"
	procs, ticks := buildProcTree(out, map[int]int{12: 100}, time.Second)
	ids := map[int]procInfo{}
	for _, p := range procs {
		ids[p.PID] = p
	}
	if _, ok := ids[10]; ok {
		t.Fatal("exec wrapper visible")
	}
	if _, ok := ids[20]; ok {
		t.Fatal("probe visible")
	}
	if ids[11].PPID != 0 {
		t.Fatalf("child of hidden wrapper not reparented: %+v", ids[11])
	}
	if ids[12].CPU != 50 {
		t.Fatalf("cpu = %v, want 50", ids[12].CPU)
	}
	if ticks[12] != 150 {
		t.Fatalf("ticks not recorded: %v", ticks)
	}
}
