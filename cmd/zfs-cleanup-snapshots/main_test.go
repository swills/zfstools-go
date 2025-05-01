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
			args: args{name: "/usr/sbin/zfs-cleanup-snapshots"},
			wantWriter: `Usage: /usr/sbin/zfs-cleanup-snapshots [-dnv]    -d              Show debug output.
    -n              Do a dry-run. Nothing is committed. Only show what would be done.
    -p              Create snapshots in parallel.
    -P pool         Act only on the specified pool.
    -v              Show what is being done.
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
