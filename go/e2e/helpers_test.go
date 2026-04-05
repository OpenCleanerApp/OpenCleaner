//go:build darwin && e2e

package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

func goRootDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), ".."))
}

func buildBinary(t *testing.T, outPath, pkg string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", outPath, pkg)
	cmd.Dir = goRootDir(t)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build %s failed: %v (%s)", pkg, err, strings.TrimSpace(stderr.String()))
	}
}

func buildBinaries(t *testing.T) (cliPath, daemonPath string) {
	t.Helper()
	outDir := t.TempDir()
	cliPath = filepath.Join(outDir, "opencleaner")
	daemonPath = filepath.Join(outDir, "opencleanerd")
	buildBinary(t, cliPath, "./cmd/opencleaner")
	buildBinary(t, daemonPath, "./cmd/opencleanerd")
	return cliPath, daemonPath
}

type cmdResult struct {
	Stdout string
	Stderr string
	Code   int
}

func runCmd(t *testing.T, env map[string]string, bin string, args ...string) cmdResult {
	t.Helper()

	timeout := 20 * time.Second
	if dl, ok := t.Deadline(); ok {
		remain := time.Until(dl) - 2*time.Second
		if remain > 0 && remain < timeout {
			timeout = remain
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = mergeEnv(env)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("command timed out: %s %v\nstdout=%s\nstderr=%s", bin, args, stdout.String(), stderr.String())
	}

	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			t.Fatalf("exec %s failed: %v", bin, err)
		}
	}
	return cmdResult{Stdout: stdout.String(), Stderr: stderr.String(), Code: code}
}

func mergeEnv(extra map[string]string) []string {
	m := map[string]string{}
	for _, kv := range os.Environ() {
		k, v, ok := strings.Cut(kv, "=")
		if ok {
			m[k] = v
		}
	}
	for k, v := range extra {
		m[k] = v
	}

	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(out)
	return out
}

func waitFor(t *testing.T, timeout time.Duration, fn func() error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := fn(); err == nil {
			return
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("timeout waiting: %v", lastErr)
	}
	t.Fatal("timeout waiting")
}

func startDaemon(t *testing.T, daemonBin string, env map[string]string, socketPath string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, daemonBin, "-socket", socketPath, "-log-level", "debug")
	cmd.Env = mergeEnv(env)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr

	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start daemon failed: %v", err)
	}

	t.Cleanup(func() {
		cancel()
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()

		select {
		case <-time.After(3 * time.Second):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			<-done
		case <-done:
		}
		_ = os.Remove(socketPath)
	})
}

func shortSocketPath(t *testing.T) string {
	t.Helper()
	p := filepath.Join("/tmp", fmt.Sprintf("opencleaner-e2e-%d-%d.sock", os.Getpid(), time.Now().UnixNano()))
	t.Cleanup(func() {
		_ = os.Remove(p)
	})
	return p
}
