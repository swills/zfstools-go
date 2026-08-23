package zfs

import (
	"errors"
	"io"
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

func (runner *fakeRunner) Run(output io.Writer, name string, args ...string) error {
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

	err := (execRunner{}).Run(&output, "sh", "-c", "printf command-output")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got := output.String(); got != "command-output" {
		t.Errorf("Run() output = %q, want command-output", got)
	}
}

func TestExecRunnerIncludesStderr(t *testing.T) {
	t.Parallel()

	err := (execRunner{}).Run(io.Discard, "sh", "-c", "echo command failed >&2; exit 1")
	if err == nil {
		t.Fatal("Run() error = nil, want command error")
	}

	if !strings.Contains(err.Error(), "command failed") {
		t.Errorf("Run() error = %q, want stderr", err)
	}
}
