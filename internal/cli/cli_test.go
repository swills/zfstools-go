package cli

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
)

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

			code := Run("/usr/local/sbin/"+name, []string{"--version"}, stdout, stderr, "1.2.3", "abc123")
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
	if code := Run("zfstools", nil, &bytes.Buffer{}, stderr, "dev", "none"); code != 2 {
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
	code := RunAutoSnapshot(
		autoSnapshotName,
		[]string{"daily", "10oops"},
		&bytes.Buffer{},
		stderr,
		"dev",
		"none",
	)

	if code != 2 {
		t.Errorf("RunAutoSnapshot() code = %d, want 2", code)
	}

	if got, want := stderr.String(), "invalid KEEP \"10oops\": must be a non-negative decimal integer\n"; got != want {
		t.Errorf("RunAutoSnapshot() stderr = %q, want %q", got, want)
	}
}

func TestRunSnapshotMySQLDryRun(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}

	code := RunSnapshotMySQL(
		mysqlName,
		[]string{"--dry-run", "--verbose", "pool/mysql"},
		stdout,
		&bytes.Buffer{},
		"dev",
		"none",
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
