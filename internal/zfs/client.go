package zfs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync/atomic"
)

// Runner executes external commands used by ZFS operations. It exposes stdout
// to callers but deliberately not stderr: implementations capture stderr and
// include it in returned errors. commandRunner is the lower-level boundary
// where both streams are supplied explicitly.
type Runner interface {
	Run(ctx context.Context, output io.Writer, name string, args ...string) error
}

// commandRunner is the raw operating-system command boundary consumed by
// execRunner. Keeping command policy above this interface makes that policy
// testable without starting a process.
type commandRunner interface {
	run(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error
}

// osCommands is the thin os/exec adapter used by the system client.
type osCommands struct{}

// run implements commandRunner by delegating one command to the standard
// library with the supplied context and output streams.
func (osCommands) run(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	//nolint:wrapcheck // execRunner owns the command error context and stderr policy.
	return cmd.Run()
}

// execRunner applies the command execution policy required by Runner.
type execRunner struct {
	commands commandRunner
}

// Run implements Runner by capturing command stderr and adding it to command
// failures while streaming stdout to the caller.
func (runner execRunner) Run(ctx context.Context, output io.Writer, name string, args ...string) error {
	var stderr bytes.Buffer

	err := runner.commands.run(ctx, output, &stderr, name, args...)
	if err != nil {
		return wrapCommandError(name, stderr.String(), err)
	}

	return nil
}

// wrapCommandError preserves the command failure while adding its name and
// any stderr emitted by the command.
func wrapCommandError(name, stderr string, err error) error {
	stderr = strings.TrimSpace(stderr)
	if stderr != "" {
		return fmt.Errorf("run %s: %w: %s", name, err, stderr)
	}

	return fmt.Errorf("run %s: %w", name, err)
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
	return NewClient(execRunner{commands: osCommands{}}, output)
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
