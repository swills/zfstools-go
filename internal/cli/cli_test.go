package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"zfstools-go/internal/zfs"
)

type fakeRunner struct {
	datasetOutput string
	calls         []cleanupCommand
	mu            sync.Mutex
	fail          bool
	failDatasets  bool
	failSnapshot  bool
	failSnapshots bool
}

type cleanupCommand struct {
	name string
	args []string
}

var errTestCommand = errors.New("test command failed")

func (runner *fakeRunner) Run(_ context.Context, output io.Writer, name string, args ...string) error {
	runner.mu.Lock()
	runner.calls = append(runner.calls, cleanupCommand{name: name, args: append([]string(nil), args...)})
	runner.mu.Unlock()

	if err := runner.commandError(name, args); err != nil {
		return err
	}

	if len(args) == 0 {
		return nil
	}

	var data string

	switch args[0] {
	case "get":
		data = "0\n"
	case "list":
		switch {
		case slices.Contains(args, "snapshot"):
			data = "tank/data@manual-new\t0\ntank/data@manual-old\t0\n"
		case runner.datasetOutput != "":
			data = runner.datasetOutput
		case strings.Contains(strings.Join(args, " "), "com.sun:auto-snapshot"):
			data = "tank/data\tfilesystem\t-\ttrue\tyes\n"
		default:
			data = "tank/data\tfilesystem\n"
		}
	}

	_, err := io.WriteString(output, data)
	if err != nil {
		return fmt.Errorf("write fake command output: %w", err)
	}

	return nil
}

func (runner *fakeRunner) commandError(name string, args []string) error {
	if runner.fail {
		return errTestCommand
	}

	if len(args) == 0 {
		return nil
	}

	isSnapshotList := args[0] == "list" && slices.Contains(args, "snapshot")
	if runner.failDatasets && args[0] == "list" && !isSnapshotList {
		return errTestCommand
	}

	if runner.failSnapshots && isSnapshotList {
		return errTestCommand
	}

	if runner.failSnapshot && name == "zfs" && args[0] == "snapshot" {
		return errTestCommand
	}

	return nil
}

func TestUsageWriters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		write func(*bytes.Buffer, string)
		want  string
	}{
		{
			name:  "/usr/local/sbin/zfs-auto-snapshot",
			write: func(writer *bytes.Buffer, name string) { writeAutoSnapshotUsage(writer, name) },
			want: `Usage: /usr/local/sbin/zfs-auto-snapshot [-dknpuv] <INTERVAL> <KEEP>
    -d              Show debug output.
    -k              Keep zero-sized snapshots.
    -n              Do a dry-run. Nothing is committed. Only show what would be done.
    -p              Create snapshots in parallel.
    -P pool         Act only on the specified pool.
    -u              Use UTC for snapshots.
    -v              Show what is being done.
    INTERVAL        The interval to snapshot.
    KEEP            How many snapshots to keep.
`,
		},
		{
			name:  "/usr/sbin/zfs-cleanup-snapshots",
			write: func(writer *bytes.Buffer, name string) { writeCleanupUsage(writer, name) },
			want: `Usage: /usr/sbin/zfs-cleanup-snapshots [-dnv]
    -d              Show debug output.
    -n              Do a dry-run. Nothing is committed. Only show what would be done.
    -p              Create snapshots in parallel.
    -P pool         Act only on the specified pool.
    -v              Show what is being done.
`,
		},
		{
			name:  "/usr/local/sbin/zfs-snapshot-mysql",
			write: func(writer *bytes.Buffer, name string) { writeSnapshotMySQLUsage(writer, name) },
			want: `Usage: /usr/local/sbin/zfs-snapshot-mysql [-dnv] DATASET
    -d              Show debug output.
    -n              Do a dry-run. Nothing is committed. Only show what would be done.
    -v              Show what is being done.
`,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			writer := &bytes.Buffer{}
			testCase.write(writer, testCase.name)

			if got := writer.String(); got != testCase.want {
				t.Errorf("usage = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestRunDispatchesByExecutableName(t *testing.T) {
	t.Parallel()

	for _, name := range []string{autoSnapshotName, cleanupName, mysqlName} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}

			code := Run(t.Context(), "/usr/local/sbin/"+name, []string{"--version"}, stdout, stderr, "1.2.3", "abc123")
			if code != 0 {
				t.Errorf("Run() code = %d, want 0; stderr = %q", code, stderr.String())
			}

			if got, want := stdout.String(), "1.2.3 (commit abc123)\n"; got != want {
				t.Errorf("Run() stdout = %q, want %q", got, want)
			}
		})
	}
}

func TestRunRejectsUnknownExecutableName(t *testing.T) {
	t.Parallel()

	stderr := &bytes.Buffer{}
	if code := Run(t.Context(), "zfstools", nil, &bytes.Buffer{}, stderr, "dev", "none"); code != 2 {
		t.Errorf("Run() code = %d, want 2", code)
	}

	if stderr.Len() == 0 {
		t.Error("Run() did not explain the unknown executable name")
	}
}

func TestParseKeep(t *testing.T) {
	t.Parallel()

	overflow := "2147483648"
	if strconv.IntSize == 64 {
		overflow = "9223372036854775808"
	}

	tests := []struct {
		name    string
		value   string
		want    int
		wantErr bool
	}{
		{name: "zero", value: "0", want: 0},
		{name: "positive", value: "10", want: 10},
		{name: "negative", value: "-1", wantErr: true},
		{name: "partial", value: "10oops", wantErr: true},
		{name: "leading whitespace", value: " 10", wantErr: true},
		{name: "trailing whitespace", value: "10 ", wantErr: true},
		{name: "empty", value: "", wantErr: true},
		{name: "overflow", value: overflow, wantErr: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseKeep(testCase.value)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("parseKeep(%q) error = %v, wantErr %t", testCase.value, err, testCase.wantErr)
			}

			if got != testCase.want {
				t.Errorf("parseKeep(%q) = %d, want %d", testCase.value, got, testCase.want)
			}
		})
	}
}

func TestRunAutoSnapshotRejectsInvalidKeep(t *testing.T) {
	t.Parallel()

	stderr := &bytes.Buffer{}
	client := zfs.NewClient(&fakeRunner{}, io.Discard)
	code := runAutoSnapshot(
		t.Context(),
		autoSnapshotName,
		[]string{"daily", "10oops"},
		&bytes.Buffer{},
		stderr,
		"dev",
		"none",
		client,
	)

	if code != 2 {
		t.Errorf("RunAutoSnapshot() code = %d, want 2", code)
	}

	if got, want := stderr.String(), "invalid KEEP \"10oops\": must be a non-negative decimal integer\n"; got != want {
		t.Errorf("RunAutoSnapshot() stderr = %q, want %q", got, want)
	}
}

func TestRunAutoSnapshot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		args      []string
		wantCalls int
	}{
		{name: "keep zero", args: []string{"--keep-zero-sized-snapshots", "daily", "0"}, wantCalls: 2},
		{name: "keep one", args: []string{"daily", "1"}, wantCalls: 4},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			runner := &fakeRunner{}
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			client := zfs.NewClient(runner, stdout)

			code := runAutoSnapshot(t.Context(), autoSnapshotName, testCase.args, stdout, stderr, "dev", "none", client)
			if code != 0 {
				t.Fatalf("runAutoSnapshot() code = %d, want 0; stderr = %q", code, stderr.String())
			}

			if len(runner.calls) != testCase.wantCalls {
				t.Errorf("Run calls = %d, want %d", len(runner.calls), testCase.wantCalls)
			}
		})
	}
}

func TestRunAutoSnapshotReportsCreationFailure(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		datasetOutput: "tank/data\tfilesystem\tmysql\ttrue\tyes\n",
		failSnapshot:  true,
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	client := zfs.NewClient(runner, stdout)

	code := runAutoSnapshot(
		t.Context(), autoSnapshotName, []string{"daily", "1"}, stdout, stderr, "dev", "none", client,
	)
	if code != 1 {
		t.Fatalf("runAutoSnapshot() code = %d, want 1", code)
	}

	if !strings.Contains(stderr.String(), "Error creating snapshots") {
		t.Errorf("runAutoSnapshot() stderr = %q, want creation error", stderr.String())
	}
}

func TestRunAutoSnapshotReportsDatasetDiscoveryFailure(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{failDatasets: true}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	client := zfs.NewClient(runner, stdout)

	code := runAutoSnapshot(
		t.Context(), autoSnapshotName, []string{"daily", "1"}, stdout, stderr, "dev", "none", client,
	)
	if code != 1 {
		t.Fatalf("runAutoSnapshot() code = %d, want 1", code)
	}

	if !strings.Contains(stderr.String(), "Error finding eligible datasets") {
		t.Errorf("runAutoSnapshot() stderr = %q, want discovery error", stderr.String())
	}

	if len(runner.calls) != 1 {
		t.Errorf("command count = %d, want dataset listing only", len(runner.calls))
	}
}

func TestRunAutoSnapshotReportsCleanupDiscoveryFailure(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{failSnapshots: true}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	client := zfs.NewClient(runner, stdout)

	code := runAutoSnapshot(
		t.Context(), autoSnapshotName, []string{"daily", "0"}, stdout, stderr, "dev", "none", client,
	)
	if code != 1 {
		t.Fatalf("runAutoSnapshot() code = %d, want 1", code)
	}

	if !strings.Contains(stderr.String(), "Error cleaning up snapshots") {
		t.Errorf("runAutoSnapshot() stderr = %q, want cleanup discovery error", stderr.String())
	}
}

func TestRunSnapshotMySQLDryRun(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	client := zfs.NewClient(&fakeRunner{}, stdout)

	code := runSnapshotMySQL(
		t.Context(),
		mysqlName,
		[]string{"--dry-run", "--verbose", "pool/mysql"},
		stdout,
		&bytes.Buffer{},
		"dev",
		"none",
		client,
	)
	if code != 0 {
		t.Fatalf("RunSnapshotMySQL() code = %d, want 0", code)
	}

	for _, value := range []string{"mysql -e", "SYSTEM zfs snapshot -r pool/mysql@"} {
		if !strings.Contains(stdout.String(), value) {
			t.Errorf("RunSnapshotMySQL() stdout = %q, want substring %q", stdout.String(), value)
		}
	}
}

func TestRunCleanupSnapshotsOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "debug short", args: []string{"-d", "--version"}},
		{name: "debug long", args: []string{"--debug", "--version"}},
		{name: "dry run short", args: []string{"-n", "--version"}},
		{name: "dry run long", args: []string{"--dry-run", "--version"}},
		{name: "parallel short", args: []string{"-p", "--version"}},
		{name: "parallel long", args: []string{"--parallel-snapshots", "--version"}},
		{name: "pool short", args: []string{"-P", "tank", "--version"}},
		{name: "pool long", args: []string{"--pool", "tank", "--version"}},
		{name: "verbose short", args: []string{"-v", "--version"}},
		{name: "verbose long", args: []string{"--verbose", "--version"}},
		{name: "combined short", args: []string{"-dnpv", "--version"}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			client := zfs.NewClient(&fakeRunner{}, stdout)

			code := runCleanupSnapshots(
				t.Context(), cleanupName, testCase.args, stdout, stderr, "1.2.3", "abc123", client,
			)
			if code != 0 {
				t.Fatalf("RunCleanupSnapshots() code = %d, want 0; stderr = %q", code, stderr.String())
			}

			if got, want := stdout.String(), "1.2.3 (commit abc123)\n"; got != want {
				t.Errorf("RunCleanupSnapshots() stdout = %q, want %q", got, want)
			}
		})
	}
}

func TestRunCleanupSnapshots(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	client := zfs.NewClient(runner, stdout)
	args := []string{"--debug", "--verbose", "--parallel-snapshots", "--pool", "tank"}

	code := runCleanupSnapshots(t.Context(), cleanupName, args, stdout, stderr, "dev", "none", client)
	if code != 0 {
		t.Fatalf("runCleanupSnapshots() code = %d, want 0; stderr = %q", code, stderr.String())
	}

	runner.mu.Lock()
	calls := append([]cleanupCommand(nil), runner.calls...)
	runner.mu.Unlock()

	var destroyed []string

	for _, call := range calls {
		if call.name == "zfs" && len(call.args) > 0 && call.args[0] == "destroy" {
			destroyed = append(destroyed, call.args[len(call.args)-1])
		}
	}

	if len(destroyed) != 1 || destroyed[0] != "tank/data@manual-old" {
		t.Errorf("destroyed snapshots = %v, want tank/data@manual-old", destroyed)
	}

	for _, want := range []string{
		"zfs list -r -H -p -t snapshot",
		"zfs list -H -t filesystem,volume",
		"Destroying zero-sized snapshot: tank/data@manual-old",
		"zfs destroy -d tank/data@manual-old",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("runCleanupSnapshots() stdout = %q, want substring %q", stdout.String(), want)
		}
	}
}

func TestRunCleanupSnapshotsReportsDatasetDiscoveryFailure(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{failDatasets: true}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	client := zfs.NewClient(runner, stdout)

	code := runCleanupSnapshots(t.Context(), cleanupName, nil, stdout, stderr, "dev", "none", client)
	if code != 1 {
		t.Fatalf("runCleanupSnapshots() code = %d, want 1", code)
	}

	if !strings.Contains(stderr.String(), "Error listing datasets") {
		t.Errorf("runCleanupSnapshots() stderr = %q, want dataset discovery error", stderr.String())
	}

	for _, call := range runner.calls {
		if call.name == "zfs" && len(call.args) > 0 && call.args[0] == "destroy" {
			t.Errorf("unexpected destroy command: %v", call.args)
		}
	}
}

func TestRunReportsParseErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command string
		want    string
		args    []string
	}{
		{name: "auto unknown long", command: autoSnapshotName, args: []string{"--bogus"}, want: "unknown flag"},
		{name: "cleanup unknown long", command: cleanupName, args: []string{"--bogus"}, want: "unknown flag"},
		{name: "mysql unknown long", command: mysqlName, args: []string{"--bogus"}, want: "unknown flag"},
		{
			name: "auto missing pool", command: autoSnapshotName, args: []string{"--pool"},
			want: "flag needs an argument",
		},
		{
			name: "cleanup missing pool", command: cleanupName, args: []string{"--pool"},
			want: "flag needs an argument",
		},
		{name: "mysql invalid shorthand", command: mysqlName, args: []string{"-x"}, want: "unknown shorthand flag"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			stderr := &bytes.Buffer{}

			code := Run(t.Context(), testCase.command, testCase.args, &bytes.Buffer{}, stderr, "dev", "none")
			if code != 2 {
				t.Errorf("Run() code = %d, want 2", code)
			}

			for _, value := range []string{testCase.want, "Usage:"} {
				if !strings.Contains(stderr.String(), value) {
					t.Errorf("Run() stderr = %q, want substring %q", stderr.String(), value)
				}
			}
		})
	}
}

func TestRunHelp(t *testing.T) {
	t.Parallel()

	for _, command := range []string{autoSnapshotName, cleanupName, mysqlName} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()

			stderr := &bytes.Buffer{}

			code := Run(t.Context(), command, []string{"--help"}, &bytes.Buffer{}, stderr, "dev", "none")
			if code != 0 {
				t.Errorf("Run() code = %d, want 0", code)
			}

			if got := strings.Count(stderr.String(), "Usage:"); got != 1 {
				t.Errorf("Run() usage count = %d, want 1; stderr = %q", got, stderr.String())
			}
		})
	}
}

func TestRunShowsUsageForInvalidArgumentCounts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command string
		args    []string
	}{
		{name: "auto missing interval and keep", command: autoSnapshotName},
		{name: "cleanup unexpected argument", command: cleanupName, args: []string{"unexpected"}},
		{name: "mysql missing dataset", command: mysqlName},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			stderr := &bytes.Buffer{}
			code := Run(t.Context(), testCase.command, testCase.args, &bytes.Buffer{}, stderr, "dev", "none")

			if code != 0 {
				t.Errorf("Run() code = %d, want 0", code)
			}

			if !strings.Contains(stderr.String(), "Usage:") {
				t.Errorf("Run() stderr = %q, want usage", stderr.String())
			}
		})
	}
}

func TestRunCleanupReportsCommandFailure(t *testing.T) {
	t.Parallel()

	assertCommandFailure(t, cleanupName, nil, "Error listing snapshots")
}

func TestRunSnapshotMySQLReportsCommandFailure(t *testing.T) {
	t.Parallel()

	assertCommandFailure(t, mysqlName, []string{"tank/mysql"}, "Error creating snapshot")
}

func assertCommandFailure(t *testing.T, command string, args []string, want string) {
	t.Helper()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	client := zfs.NewClient(&fakeRunner{fail: true}, stdout)
	code := run(t.Context(), command, args, stdout, stderr, "dev", "none", client)

	if code != 1 {
		t.Errorf("Run() code = %d, want 1", code)
	}

	if !strings.Contains(stderr.String(), want) {
		t.Errorf("Run() stderr = %q, want substring %q", stderr.String(), want)
	}
}
