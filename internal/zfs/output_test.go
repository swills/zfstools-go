package zfs

import (
	"bytes"
	"testing"
)

func TestClientDiagnosticOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(Client)
		name string
		want string
	}{
		{
			name: "datasets",
			run: func(client Client) {
				client.ListDatasets("", nil, true)
			},
			want: "zfs list -H -t filesystem,volume -o name,type -s name\n",
		},
		{
			name: "pool feature",
			run: func(client Client) {
				_, _ = client.ListPools("", []string{"feature@bookmarks"}, true)
			},
			want: "zpool get -H -p -o name,property,value feature@bookmarks 2>/dev/null\n",
		},
		{
			name: "snapshots",
			run: func(client Client) {
				_, _ = client.ListSnapshots("tank", true, true)
			},
			want: "zfs list -r -H -p -t snapshot -o name,used -S name tank\n",
		},
		{
			name: "create snapshot",
			run: func(client Client) {
				_ = client.CreateSnapshots([]string{"pool/fs"}, "snap", false, "", true, true, false)
			},
			want: "zfs snapshot pool/fs@snap\n",
		},
		{
			name: "destroy snapshot",
			run: func(client Client) {
				_ = client.DestroySnapshot("pool/fs@snap", true, true)
			},
			want: "zfs destroy -d pool/fs@snap\n",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			output := &bytes.Buffer{}
			testCase.run(NewClient(&fakeRunner{}, output))

			if got := output.String(); got != testCase.want {
				t.Errorf("output = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestSnapshotGetUsedDiagnosticOutput(t *testing.T) {
	t.Parallel()

	output := &bytes.Buffer{}
	snapshot := Snapshot{
		Name: "pool/fs@snap", runner: &fakeRunner{output: []byte("0\n")},
		output: output, state: &snapshotState{},
	}
	snapshot.GetUsed(true)

	want := "zfs get -Hp -o value used pool/fs@snap\n"
	if got := output.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}
