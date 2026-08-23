package zfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var errTestCommand = errors.New("test command failed")

type commandCall struct {
	name string
	args []string
}

type fakeRunner struct {
	runFunc func(string, ...string) ([]byte, error)
	output  []byte
	err     error
	calls   []commandCall
	mu      sync.Mutex
}

type canceledRunner struct{}

func (canceledRunner) Run(ctx context.Context, _ io.Writer, _ string, _ ...string) error {
	return fmt.Errorf("run canceled command: %w", ctx.Err())
}

func (runner *fakeRunner) Run(_ context.Context, output io.Writer, name string, args ...string) error {
	runner.mu.Lock()
	runner.calls = append(runner.calls, commandCall{name: name, args: append([]string(nil), args...)})
	data := runner.output
	err := runner.err
	runFunc := runner.runFunc
	runner.mu.Unlock()

	if runFunc != nil {
		data, err = runFunc(name, args...)
	}

	_, writeErr := output.Write(data)

	return errors.Join(err, writeErr)
}

func TestExecRunnerWritesStdout(t *testing.T) {
	t.Parallel()

	var output strings.Builder

	err := (execRunner{}).Run(t.Context(), &output, "sh", "-c", "printf command-output")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got := output.String(); got != "command-output" {
		t.Errorf("Run() output = %q, want command-output", got)
	}
}

func TestExecRunnerIncludesStderr(t *testing.T) {
	t.Parallel()

	err := (execRunner{}).Run(t.Context(), io.Discard, "sh", "-c", "echo command failed >&2; exit 1")
	if err == nil {
		t.Fatal("Run() error = nil, want command error")
	}

	if !strings.Contains(err.Error(), "command failed") {
		t.Errorf("Run() error = %q, want stderr", err)
	}
}

func TestExecRunnerReportsStartError(t *testing.T) {
	t.Parallel()

	name := filepath.Join(t.TempDir(), "missing-command")

	err := (execRunner{}).Run(t.Context(), io.Discard, name)
	if err == nil {
		t.Fatal("Run() error = nil, want start error")
	}

	if !strings.Contains(err.Error(), name) {
		t.Errorf("Run() error = %q, want command name %q", err, name)
	}
}

func TestClientForwardsContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := NewClient(canceledRunner{}, io.Discard).ListSnapshots(ctx, "", false, false)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("ListSnapshots() error = %v, want context cancellation", err)
	}
}
