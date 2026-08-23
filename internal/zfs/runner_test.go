package zfs

import (
	"errors"
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

func (runner *fakeRunner) Run(name string, args ...string) ([]byte, error) {
	runner.mu.Lock()
	runner.calls = append(runner.calls, commandCall{name: name, args: append([]string(nil), args...)})
	runner.mu.Unlock()

	if runner.runFunc != nil {
		return runner.runFunc(name, args...)
	}

	return runner.output, runner.err
}

func TestExecRunnerIncludesStderr(t *testing.T) {
	t.Parallel()

	_, err := (execRunner{}).Run("sh", "-c", "echo command failed >&2; exit 1")
	if err == nil {
		t.Fatal("Run() error = nil, want command error")
	}

	if !strings.Contains(err.Error(), "command failed") {
		t.Errorf("Run() error = %q, want stderr", err)
	}
}
