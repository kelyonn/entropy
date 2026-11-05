package labs

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Runner executes scripts in one disposable sandbox.
type Runner interface {
	Run(ctx context.Context, script string) (output string, exitCode int, err error)
	Close()
}

// RunnerFactory creates a fresh sandbox for one validation.
type RunnerFactory func(ctx context.Context) (Runner, error)

// Report is the outcome of running a lab in a sandbox.
type Report struct {
	OK          bool            `json:"ok"`
	Problems    []string        `json:"problems,omitempty"`
	AfterSetup  map[string]bool `json:"afterSetup,omitempty"`
	AfterFix    map[string]bool `json:"afterFix,omitempty"`
	SetupOutput string          `json:"setupOutput,omitempty"`
}

// ValidateInSandbox proves a lab is real: setup must succeed, at least one
// state objective must fail right after setup, and every objective must pass
// after the reference fix. Each line of the fix counts as a learner command
// for command-based objectives.
func ValidateInSandbox(ctx context.Context, newRunner RunnerFactory, l Lab, fix string, settle time.Duration) Report {
	r := Report{}
	runner, err := newRunner(ctx)
	if err != nil {
		r.Problems = append(r.Problems, "could not start a validation sandbox: "+err.Error())
		return r
	}
	defer runner.Close()

	if _, _, err := runner.Run(ctx, "mkdir -p /root/.entropy"); err != nil {
		r.Problems = append(r.Problems, "sandbox exec failed: "+err.Error())
		return r
	}
	out, code, err := runner.Run(ctx, l.Setup)
	r.SetupOutput = tailString(out, 1500)
	if err != nil || code != 0 {
		r.Problems = append(r.Problems, fmt.Sprintf("setup exited %d: %s", code, lastNonEmpty(out)))
		return r
	}
	sleep(ctx, settle)
	before, _, _ := runner.Run(ctx, CheckScript(l))
	r.AfterSetup = Evaluate(l, before, nil)
	brokenState := false
	for _, o := range l.Objectives {
		if o.Check != "" && !r.AfterSetup[o.ID] {
			brokenState = true
		}
	}
	if !brokenState {
		r.Problems = append(r.Problems, "every state objective already passes after setup; the lab is not broken")
	}
	if fixOut, fixCode, err := runner.Run(ctx, fix); err != nil {
		r.Problems = append(r.Problems, "reference fix could not run: "+err.Error())
	} else if fixCode != 0 {
		r.Problems = append(r.Problems, fmt.Sprintf("reference fix exited %d: %s", fixCode, lastNonEmpty(fixOut)))
	}
	sleep(ctx, settle)
	after, _, _ := runner.Run(ctx, CheckScript(l))
	r.AfterFix = Evaluate(l, after, strings.Split(fix, "\n"))
	for _, o := range l.Objectives {
		if !r.AfterFix[o.ID] {
			r.Problems = append(r.Problems, fmt.Sprintf("objective %q still fails after the reference fix", o.ID))
		}
	}
	r.OK = len(r.Problems) == 0
	return r
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func tailString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func lastNonEmpty(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// DockerRunner runs a sandbox with the Docker CLI using the same limits as
// Airlock. It backs the smoke test, labctl, and CI.
type DockerRunner struct {
	id string
}

// NewDockerRunnerFactory returns a factory for image and optional runtime.
func NewDockerRunnerFactory(image, runtime string) RunnerFactory {
	return func(ctx context.Context) (Runner, error) {
		args := []string{"run", "-d", "--rm"}
		if runtime != "" {
			args = append(args, "--runtime", runtime)
		}
		args = append(args,
			"--memory", "256m", "--memory-swap", "256m", "--cpus", "0.5", "--pids-limit", "100",
			"--network", "none", "--cap-drop", "ALL",
			"--cap-add", "CHOWN", "--cap-add", "SETUID", "--cap-add", "SETGID",
			"--ulimit", "nofile=1024:2048",
			"--security-opt", "no-new-privileges:true",
			"--tmpfs", "/tmp:rw,noexec,nosuid,size=256m",
			"--tmpfs", "/var/log:rw,noexec,nosuid,size=64m",
			"-t", "-i", image, "/bin/bash")
		out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("docker run: %v: %s", err, strings.TrimSpace(string(out)))
		}
		return &DockerRunner{id: strings.TrimSpace(string(out))}, nil
	}
}

func (d *DockerRunner) Run(ctx context.Context, script string) (string, int, error) {
	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", d.id, "/bin/bash", "-s")
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); ok {
		return string(out), ee.ExitCode(), nil
	}
	return string(out), 0, err
}

func (d *DockerRunner) Close() {
	_ = exec.Command("docker", "stop", "-t", "0", d.id).Run()
}
