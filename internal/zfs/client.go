package zfs

import (
	"bytes"
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
	Run(ctx context.Context, output io.Writer, name string, args ...string) error
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, output io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = output

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if exitError, ok := errors.AsType[*exec.ExitError](err); ok && stderr.Len() > 0 {
			return fmt.Errorf("run %s: %w: %s", name, exitError, strings.TrimSpace(stderr.String()))
		}

		return fmt.Errorf("run %s: %w", name, err)
	}

	return nil
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

func (client Client) stream(ctx context.Context, name string, args ...string) (*io.PipeReader, <-chan error) {
	reader, writer := io.Pipe()
	done := make(chan error, 1)

	go client.writeStream(ctx, writer, done, name, args...)

	return reader, done
}

func (client Client) writeStream(
	ctx context.Context,
	writer *io.PipeWriter,
	done chan<- error,
	name string,
	args ...string,
) {
	err := client.runner.Run(ctx, writer, name, args...)
	_ = writer.Close()

	done <- err
}
