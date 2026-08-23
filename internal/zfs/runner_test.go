package zfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
)

var (
	errContextNotCanceled = errors.New("context was not canceled")
	errTestCommand        = errors.New("test command failed")
)

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

type fakeCommands struct {
	output          string
	stderr          string
	name            string
	args            []string
	fail            bool
	requireCanceled bool
}

func (canceledRunner) Run(ctx context.Context, _ io.Writer, _ string, _ ...string) error {
	return fmt.Errorf("run canceled command: %w", ctx.Err())
}

func (commands *fakeCommands) run(
	ctx context.Context,
	stdout, stderr io.Writer,
	name string,
	args ...string,
) error {
	commands.name = name

	commands.args = append([]string(nil), args...)

	if commands.requireCanceled && ctx.Err() == nil {
		return errContextNotCanceled
	}

	_, stdoutErr := io.WriteString(stdout, commands.output)
	_, stderrErr := io.WriteString(stderr, commands.stderr)

	var commandErr error
	if commands.fail {
		commandErr = errTestCommand
	}

	return errors.Join(commandErr, stdoutErr, stderrErr)
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

	commands := &fakeCommands{output: "command-output"}

	var output strings.Builder

	err := (execRunner{commands: commands}).Run(t.Context(), &output, "zfs", "list", "-H")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got := output.String(); got != "command-output" {
		t.Errorf("Run() output = %q, want command-output", got)
	}

	want := commandCall{name: "zfs", args: []string{"list", "-H"}}
	if commands.name != want.name || !slices.Equal(commands.args, want.args) {
		t.Errorf("command = %#v, want %#v", commandCall{name: commands.name, args: commands.args}, want)
	}
}

func TestExecRunnerIncludesStderr(t *testing.T) {
	t.Parallel()

	commands := &fakeCommands{stderr: "  command failed\n", fail: true}

	err := (execRunner{commands: commands}).Run(t.Context(), io.Discard, "zfs", "list")
	if err == nil {
		t.Fatal("Run() error = nil, want command error")
	}

	if !errors.Is(err, errTestCommand) {
		t.Errorf("Run() error = %v, want underlying command error", err)
	}

	if !strings.Contains(err.Error(), "command failed") {
		t.Errorf("Run() error = %q, want stderr", err)
	}
}

func TestExecRunnerIncludesNameWithoutStderr(t *testing.T) {
	t.Parallel()

	commands := &fakeCommands{fail: true}

	err := (execRunner{commands: commands}).Run(t.Context(), io.Discard, "missing-command")
	if err == nil {
		t.Fatal("Run() error = nil, want command error")
	}

	if !strings.Contains(err.Error(), "missing-command") {
		t.Errorf("Run() error = %q, want command name", err)
	}
}

func TestExecRunnerForwardsContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	commands := &fakeCommands{requireCanceled: true}
	if err := (execRunner{commands: commands}).Run(ctx, io.Discard, "zfs", "list"); err != nil {
		t.Fatalf("Run() error = %v", err)
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
