package zfs

import (
	"context"
	"fmt"
	"io"
	"os/exec"
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
	hasMultiSnap  func(bool) bool
}

// NewClient creates a ZFS client using the supplied command runner.
func NewClient(runner Runner, output io.Writer) Client {
	client := Client{runner: runner, output: output, snapshotState: &snapshotState{}}
	detector := &featureDetector{}
	client.hasMultiSnap = func(debug bool) bool {
		return detector.hasMultiSnap(client.ListPools, debug)
	}

	return client
}

// NewSystemClient creates a ZFS client backed by operating-system commands.
func NewSystemClient(output io.Writer) Client {
	return NewClient(execRunner{}, output)
}
