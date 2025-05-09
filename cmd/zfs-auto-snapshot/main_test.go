package main

import (
	"bytes"
	"testing"
)

func Test_usageWriter(t *testing.T) {
	type args struct {
		name string
	}

	tests := []struct {
		name       string
		args       args
		wantWriter string
	}{
		{
			name: "simple",
			args: args{name: "/usr/local/sbin/zfs-auto-snapshot"},
			wantWriter: `Usage: /usr/local/sbin/zfs-auto-snapshot [-dknpuv] <INTERVAL> <KEEP>
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
	}

	t.Parallel()

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			writer := &bytes.Buffer{}
			usageWriter(writer, testCase.args.name)

			gotWriter := writer.String()
			if gotWriter != testCase.wantWriter {
				t.Errorf("usageWriter() = %v, want %v", gotWriter, testCase.wantWriter)
			}
		})
	}
}
