package zfs

import (
	"errors"
	"sync"
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
