package zfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync/atomic"
)

// Runner executes external commands used by ZFS operations.
type Runner interface {
	Run(name string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(name string, args ...string) ([]byte, error) {
	output, err := exec.CommandContext(context.Background(), name, args...).Output()
	if err != nil {
		if exitError, ok := errors.AsType[*exec.ExitError](err); ok && len(exitError.Stderr) > 0 {
			return nil, fmt.Errorf("run %s: %w: %s", name, err, strings.TrimSpace(string(exitError.Stderr)))
		}

		return nil, fmt.Errorf("run %s: %w", name, err)
	}

	return output, nil
}

type snapshotState struct {
	stale atomic.Bool
}

type Client struct {
	runner        Runner
	output        io.Writer
	snapshotState *snapshotState
	featureCache  *featureCache
}

// NewClient creates a ZFS client using the supplied command runner.
func NewClient(runner Runner, output io.Writer) Client {
	return Client{
		runner: runner, output: output, snapshotState: &snapshotState{}, featureCache: &featureCache{},
	}
}

// NewSystemClient creates a ZFS client backed by operating-system commands.
func NewSystemClient(output io.Writer) Client {
	return NewClient(execRunner{}, output)
}
