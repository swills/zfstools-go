package zfs

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/go-test/deep"

	"zfstools-go/pkg/zfstoolstest"
)

var _ = exec.Command

//nolint:paralleltest
func TestSnapshot_GetUsed(t *testing.T) {
	type fields struct {
		Name string
		Used int64
	}

	type args struct {
		debug bool
	}

	tests := []struct {
		name        string
		mockCmdFunc string
		fields      fields
		want        int64
		args        args
		stale       bool
	}{
		{
			name:        "stale",
			mockCmdFunc: "TestSnapshot_GetUsedStale",
			fields: fields{
				Name: "pool/fs@snap",
				Used: 2048,
			},
			stale: true,
			want:  4096,
		},
		{
			name:        "notStale",
			mockCmdFunc: "TestSnapshot_GetUsedStale", // not used
			fields: fields{
				Name: "pool/fs@snap",
				Used: 1024,
			},
			stale: false,
			want:  1024,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			staleSnapshotSize = testCase.stale
			runZfsFn = zfstoolstest.MakeFakeCommand(testCase.mockCmdFunc)

			s := &Snapshot{
				Name: testCase.fields.Name,
				Used: testCase.fields.Used,
			}

			got := s.GetUsed(testCase.args.debug)
			if got != testCase.want {
				t.Errorf("GetUsed() = %v, want %v", got, testCase.want)
			}
		})
	}
}

//nolint:paralleltest
func TestIsZero(t *testing.T) {
	staleSnapshotSize = false
	runZfsFn = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command("")
	}

	snap := Snapshot{Name: "pool/fs@snap", Used: 0}
	if !snap.IsZero(false) {
		t.Error("expected IsZero to return true for Used=0")
	}

	snap.Used = 123
	if snap.IsZero(false) {
		t.Error("expected IsZero to return false for Used=123")
	}
}

//nolint:paralleltest
func TestDestroySnapshot_DryRun(t *testing.T) {
	var ran bool

	runZfsFn = func(_ string, _ ...string) *exec.Cmd {
		ran = true

		return exec.Command("false")
	}

	staleSnapshotSize = false
	DestroySnapshot("pool/fs@snap", true, false)

	if ran {
		t.Error("expected no command to run in dry-run mode")
	}
}

//nolint:paralleltest
func TestDestroySnapshot_Real(t *testing.T) {
	runZfsFn = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command("echo")
	}

	staleSnapshotSize = false
	DestroySnapshot("pool/fs@snap", false, false)

	if !staleSnapshotSize {
		t.Error("expected staleSnapshotSize = true after successful destroy")
	}
}

//nolint:paralleltest
func TestListSnapshots(t *testing.T) {
	runZfsFn = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command("echo", "pool/fs@a\t1024\n"+
			"pool/fs@b\t0")
	}

	snaps, err := ListSnapshotsFn("", false, false)
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}

	if len(snaps) != 2 {
		t.Errorf("expected 2 snapshots, got %d", len(snaps))
	}

	if snaps[0].Used != 1024 {
		t.Errorf("expected Used=1024 for first snapshot, got %d", snaps[0].Used)
	}
}

//nolint:paralleltest
func TestCreate(t *testing.T) {
	var ran bool

	runZfsFn = func(_ string, _ ...string) *exec.Cmd {
		ran = true

		return exec.Command("echo")
	}

	Create([]string{"pool/fs@snap"}, false, "", false, true, true)

	if !ran {
		t.Error("expected zfs snapshot to run")
	}
}

//nolint:paralleltest
func TestCreateMany(t *testing.T) {
	count := 0
	runZfsFn = func(_ string, _ ...string) *exec.Cmd {
		count++

		return exec.Command("echo")
	}

	CreateMany("auto-2025-01-01", []Dataset{
		{Name: "pool/fs@a"},
		{Name: "pool/fs@b"},
	}, false, false, true, true, false)

	if count != 2 {
		t.Errorf("expected 2 snapshots to be created, got %d", count)
	}
}

func Test_toIntPrefix(t *testing.T) {
	type args struct {
		s string
	}

	tests := []struct {
		name string
		args args
		want int64
	}{
		{
			name: "empty",
			args: args{s: ""},
			want: 0,
		},
		{
			name: "zero",
			args: args{s: "0"},
			want: 0,
		},
		{
			name: "zeroB",
			args: args{s: "0B"},
			want: 0,
		},
		{
			name: "numK",
			args: args{s: "123K"},
			want: 123,
		},
		{
			name: "numG",
			args: args{s: "234G"},
			want: 234,
		},
	}

	t.Parallel()

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := toIntPrefix(testCase.args.s)
			if got != testCase.want {
				t.Errorf("toIntPrefix() = %v, want %v", got, testCase.want)
			}
		})
	}
}

// test helpers from here down

//nolint:paralleltest
func TestSnapshot_GetUsedStale(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"zfs",
		"get",
		"-Hp",
		"-o",
		"value",
		"used",
		"pool/fs@snap",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	fmt.Printf("4096\n") //nolint:forbidigo

	os.Exit(0)
}
