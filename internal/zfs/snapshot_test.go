package zfs

import (
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/go-test/deep"
)

func TestSnapshotGetUsed(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{output: []byte("4096\n")}

	for _, testCase := range []struct {
		name  string
		used  int64
		stale bool
		want  int64
		calls int
	}{
		{name: "cached", used: 1024, want: 1024},
		{name: "zero", want: 4096, calls: 1},
		{name: "stale", used: 2048, stale: true, want: 4096, calls: 1},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			localState := &snapshotState{}
			localState.stale.Store(testCase.stale)

			localRunner := &fakeRunner{output: runner.output}
			snapshot := Snapshot{Name: "pool/fs@snap", Used: testCase.used, runner: localRunner, state: localState}

			if got := snapshot.GetUsed(false); got != testCase.want {
				t.Errorf("GetUsed() = %d, want %d", got, testCase.want)
			}

			if len(localRunner.calls) != testCase.calls {
				t.Errorf("Run calls = %d, want %d", len(localRunner.calls), testCase.calls)
			}

			if testCase.calls == 1 {
				want := commandCall{name: "zfs", args: []string{"get", "-Hp", "-o", "value", "used", "pool/fs@snap"}}
				if diff := deep.Equal(localRunner.calls[0], want); diff != nil {
					t.Errorf("command differs: %v", diff)
				}
			}
		})
	}
}

func TestSnapshotGetUsedErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err    error
		name   string
		output string
	}{
		{name: "command error", err: errTestCommand},
		{name: "invalid size", output: "invalid\n"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			runner := &fakeRunner{output: []byte(testCase.output), err: testCase.err}

			snapshot := Snapshot{Name: "pool/fs@snap", runner: runner, state: &snapshotState{}}
			if got := snapshot.GetUsed(false); got != 0 {
				t.Errorf("GetUsed() = %d, want 0", got)
			}
		})
	}
}

func TestSnapshotIsZero(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{output: []byte("0\n")}

	snapshot := Snapshot{Name: "pool/fs@snap", runner: runner, state: &snapshotState{}}
	if !snapshot.IsZero(false) {
		t.Fatal("IsZero() = false, want true")
	}
}

func TestListSnapshots(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		dataset   string
		output    string
		wantArgs  []string
		want      []Snapshot
		recursive bool
	}{
		{
			name:     "all",
			output:   "tank/data@backup\t134217728\n",
			wantArgs: []string{"list", "-H", "-p", "-t", "snapshot", "-o", "name,used", "-S", "name"},
			want:     []Snapshot{{Name: "tank/data@backup", Used: 134217728}},
		},
		{
			name:      "named recursive",
			dataset:   "tank",
			recursive: true,
			output:    "tank/data@backup1\t131072\n",
			wantArgs:  []string{"list", "-r", "-H", "-p", "-t", "snapshot", "-o", "name,used", "-S", "name", "tank"},
			want:      []Snapshot{{Name: "tank/data@backup1", Used: 131072}},
		},
		{
			name:     "named non-recursive",
			dataset:  "tank",
			wantArgs: []string{"list", "-d", "1", "-H", "-p", "-t", "snapshot", "-o", "name,used", "-S", "name", "tank"},
			want:     []Snapshot{},
		},
		{
			name:     "malformed rows",
			output:   "invalid\ninvalid@size\tnot-a-number\n",
			wantArgs: []string{"list", "-H", "-p", "-t", "snapshot", "-o", "name,used", "-S", "name"},
			want:     []Snapshot{},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			runner := &fakeRunner{output: []byte(testCase.output)}

			got, err := NewClient(runner, io.Discard).ListSnapshots(testCase.dataset, testCase.recursive, false)
			if err != nil {
				t.Fatalf("ListSnapshots() error = %v", err)
			}

			publicGot := make([]Snapshot, len(got))
			for i := range got {
				publicGot[i] = Snapshot{Name: got[i].Name, Used: got[i].Used}
			}

			if diff := deep.Equal(publicGot, testCase.want); diff != nil {
				t.Errorf("snapshots differ: %v", diff)
			}

			wantCall := []commandCall{{name: "zfs", args: testCase.wantArgs}}
			if diff := deep.Equal(runner.calls, wantCall); diff != nil {
				t.Errorf("command differs: %v", diff)
			}
		})
	}
}

func TestListSnapshotsCommandError(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{err: errTestCommand}
	if _, err := NewClient(runner, io.Discard).ListSnapshots("", false, false); err == nil {
		t.Fatal("ListSnapshots() error = nil, want command error")
	}
}

func TestCreateSnapshot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr   error
		name      string
		database  string
		wantCmd   string
		targets   []string
		recursive bool
		dryRun    bool
	}{
		{name: "no targets", wantErr: ErrEmptySnapshotName},
		{name: "invalid target", targets: []string{"pool/fs"}, wantErr: ErrInvalidSnapshotName},
		{name: "single", targets: []string{"pool/fs@snap"}, wantCmd: "zfs snapshot pool/fs@snap"},
		{
			name: "multiple recursive", targets: []string{"pool/fs1@snap", "pool/fs2@snap"}, recursive: true,
			wantCmd: "zfs snapshot -r pool/fs1@snap pool/fs2@snap",
		},
		{
			name: "mysql", targets: []string{"pool/fs@snap"}, database: "mysql",
			wantCmd: "mysql -e \" FLUSH LOGS; FLUSH TABLES WITH READ LOCK; " +
				"SYSTEM zfs snapshot pool/fs@snap; UNLOCK TABLES;\"",
		},
		{
			name: "postgresql", targets: []string{"pool/fs@snap"}, database: "postgresql",
			wantCmd: "(psql -c \"SELECT PG_START_BACKUP('zfs-auto-snapshot');\" postgres ; " +
				"zfs snapshot pool/fs@snap ) ; psql -c \"SELECT PG_STOP_BACKUP();\" postgres",
		},
		{name: "dry run", targets: []string{"pool/fs@snap"}, dryRun: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			runner := &fakeRunner{}
			client := NewClient(runner, io.Discard)

			err := client.CreateSnapshot(testCase.targets, testCase.recursive, testCase.database,
				testCase.dryRun, false, false)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("createSnapshot() error = %v, want %v", err, testCase.wantErr)
			}

			wantCalls := []commandCall(nil)
			if testCase.wantCmd != "" && !testCase.dryRun {
				wantCalls = []commandCall{{name: "sh", args: []string{"-c", testCase.wantCmd}}}
			}

			if diff := deep.Equal(runner.calls, wantCalls); diff != nil {
				t.Errorf("commands differ: %v", diff)
			}
		})
	}
}

func TestCreateSnapshotCommandError(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{err: errTestCommand}
	if err := NewClient(runner, io.Discard).CreateSnapshot(
		[]string{"pool/fs@snap"}, false, "", false, false, false,
	); err == nil {
		t.Fatal("createSnapshot() error = nil, want command error")
	}
}

func TestCreateManySnapshots(t *testing.T) {
	t.Parallel()

	t.Run("multi-snapshot", func(t *testing.T) {
		t.Parallel()

		runner := &fakeRunner{output: []byte("123456\n")}
		client := NewClient(runner, io.Discard)
		client.hasMultiSnap = func(bool) bool { return true }

		err := client.CreateManySnapshots("auto",
			[]Dataset{{Name: "pool/fs1"}, {Name: "pool/fs2"}}, false, false, false, false, false)
		if err != nil {
			t.Fatalf("createManySnapshots() error = %v", err)
		}

		want := []commandCall{
			{name: "getconf", args: []string{"ARG_MAX"}},
			{name: "sh", args: []string{"-c", "zfs snapshot pool/fs1@auto pool/fs2@auto"}},
		}

		if diff := deep.Equal(runner.calls, want); diff != nil {
			t.Errorf("commands differ: %v", diff)
		}
	})

	t.Run("single snapshots", func(t *testing.T) {
		t.Parallel()

		runner := &fakeRunner{}
		client := NewClient(runner, io.Discard)
		client.hasMultiSnap = func(bool) bool { return false }

		err := client.CreateManySnapshots("auto",
			[]Dataset{{Name: "pool/fs1"}, {Name: "pool/fs2"}}, false, false, false, false, false)
		if err != nil {
			t.Fatalf("createManySnapshots() error = %v", err)
		}

		want := []commandCall{
			{name: "sh", args: []string{"-c", "zfs snapshot pool/fs1@auto"}},
			{name: "sh", args: []string{"-c", "zfs snapshot pool/fs2@auto"}},
		}
		if diff := deep.Equal(runner.calls, want); diff != nil {
			t.Errorf("commands differ: %v", diff)
		}
	})

	t.Run("one command fails", func(t *testing.T) {
		t.Parallel()

		runner := &fakeRunner{runFunc: func(_ string, args ...string) ([]byte, error) {
			if args[1] == "zfs snapshot pool/fs2@auto" {
				return nil, errTestCommand
			}

			return nil, nil
		}}
		client := NewClient(runner, io.Discard)
		client.hasMultiSnap = func(bool) bool { return false }

		err := client.CreateManySnapshots("auto",
			[]Dataset{{Name: "pool/fs1"}, {Name: "pool/fs2"}}, false, false, false, false, false)
		if !errors.Is(err, ErrOneSnapshotOfManyErrored) {
			t.Fatalf("createManySnapshots() error = %v, want %v", err, ErrOneSnapshotOfManyErrored)
		}
	})
}

func TestCreateManySnapshotsDatabaseDataset(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	client := NewClient(runner, io.Discard)
	client.hasMultiSnap = func(bool) bool {
		t.Fatal("database-only request checked multi-snapshot support")

		return false
	}

	err := client.CreateManySnapshots(
		"auto", []Dataset{{Name: "pool/mysql", DB: "mysql"}}, false, false, false, false, false,
	)
	if err != nil {
		t.Fatalf("CreateManySnapshots() error = %v", err)
	}

	want := []commandCall{{name: "sh", args: []string{
		"-c", "mysql -e \" FLUSH LOGS; FLUSH TABLES WITH READ LOCK; " +
			"SYSTEM zfs snapshot pool/mysql@auto; UNLOCK TABLES;\"",
	}}}
	if diff := deep.Equal(runner.calls, want); diff != nil {
		t.Errorf("commands differ: %v", diff)
	}
}

func TestCreateManySnapshotsMixedDatasetsContinueAfterError(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{runFunc: func(_ string, args ...string) ([]byte, error) {
		if strings.HasPrefix(args[1], "mysql -e") {
			return nil, errTestCommand
		}

		return nil, nil
	}}
	client := NewClient(runner, io.Discard)
	client.hasMultiSnap = func(bool) bool { return false }

	err := client.CreateManySnapshots("auto", []Dataset{
		{Name: "pool/mysql", DB: "mysql"},
		{Name: "pool/files"},
	}, false, false, false, false, false)
	if !errors.Is(err, ErrOneSnapshotOfManyErrored) {
		t.Fatalf("CreateManySnapshots() error = %v, want %v", err, ErrOneSnapshotOfManyErrored)
	}

	if len(runner.calls) != 2 {
		t.Fatalf("command count = %d, want 2", len(runner.calls))
	}

	if got := runner.calls[1]; deep.Equal(got, commandCall{
		name: "sh", args: []string{"-c", "zfs snapshot pool/files@auto"},
	}) != nil {
		t.Errorf("regular dataset command = %#v", got)
	}
}

func TestCreateManySnapshotsParallelFallback(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name    string
		fail    string
		wantErr bool
	}{
		{name: "success"},
		{name: "failure", fail: "zfs snapshot pool/fs2@auto", wantErr: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			runner := &fakeRunner{runFunc: func(_ string, args ...string) ([]byte, error) {
				if args[1] == testCase.fail {
					return nil, errTestCommand
				}

				return nil, nil
			}}
			client := NewClient(runner, io.Discard)
			client.hasMultiSnap = func(bool) bool { return false }

			err := client.CreateManySnapshots("auto", []Dataset{
				{Name: "pool/fs1"}, {Name: "pool/fs2"}, {Name: "pool/fs3"},
			}, false, false, false, false, true)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("CreateManySnapshots() error = %v, wantErr %v", err, testCase.wantErr)
			}

			commands := make([]string, 0, len(runner.calls))
			for _, call := range runner.calls {
				if call.name != "sh" || len(call.args) != 2 || call.args[0] != "-c" {
					t.Fatalf("unexpected command: %#v", call)
				}

				commands = append(commands, call.args[1])
			}

			slices.Sort(commands)

			want := []string{
				"zfs snapshot pool/fs1@auto",
				"zfs snapshot pool/fs2@auto",
				"zfs snapshot pool/fs3@auto",
			}
			if diff := deep.Equal(commands, want); diff != nil {
				t.Errorf("commands differ: %v", diff)
			}
		})
	}
}

func TestCreateManySnapshotsMinimumChunkSize(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{output: []byte("1\n")}
	client := NewClient(runner, io.Discard)
	client.hasMultiSnap = func(bool) bool { return true }

	err := client.CreateManySnapshots("auto", []Dataset{
		{Name: "pool/fs1"}, {Name: "pool/fs2"},
	}, false, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateManySnapshots() error = %v", err)
	}

	want := []commandCall{
		{name: "getconf", args: []string{"ARG_MAX"}},
		{name: "sh", args: []string{"-c", "zfs snapshot pool/fs1@auto"}},
		{name: "sh", args: []string{"-c", "zfs snapshot pool/fs2@auto"}},
	}
	if diff := deep.Equal(runner.calls, want); diff != nil {
		t.Errorf("commands differ: %v", diff)
	}
}

func TestCreateManySnapshotsValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		want     error
		name     string
		snapshot string
		datasets []Dataset
	}{
		{name: "empty snapshot", datasets: []Dataset{{Name: "pool/fs"}}, want: ErrEmptySnapshotName},
		{name: "no datasets", snapshot: "auto", want: ErrNoDatasets},
		{name: "empty dataset", snapshot: "auto", datasets: []Dataset{{Name: ""}}, want: ErrInvalidSnapshotName},
		{
			name: "snapshot in dataset", snapshot: "auto",
			datasets: []Dataset{{Name: "pool/fs@old"}}, want: ErrInvalidSnapshotName,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			client := NewClient(&fakeRunner{}, io.Discard)
			client.hasMultiSnap = func(bool) bool { return false }

			err := client.CreateManySnapshots(testCase.snapshot,
				testCase.datasets, false, false, false, false, false)
			if !errors.Is(err, testCase.want) {
				t.Errorf("createManySnapshots() error = %v, want %v", err, testCase.want)
			}
		})
	}
}

func TestGetArgMax(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err    error
		name   string
		output string
		want   int
	}{
		{name: "value", output: "123456\n", want: 123456},
		{name: "command error", err: errTestCommand, want: 4096},
		{name: "invalid value", output: "bogus\n", want: 4096},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			runner := &fakeRunner{output: []byte(testCase.output), err: testCase.err}
			if got := NewClient(runner, io.Discard).getArgMax(); got != testCase.want {
				t.Errorf("getArgMax() = %d, want %d", got, testCase.want)
			}

			want := []commandCall{{name: "getconf", args: []string{"ARG_MAX"}}}
			if diff := deep.Equal(runner.calls, want); diff != nil {
				t.Errorf("command differs: %v", diff)
			}
		})
	}
}

func TestDestroySnapshotInvalidatesClientSnapshots(t *testing.T) {
	t.Parallel()

	var listDone bool

	runner := &fakeRunner{
		runFunc: func(string, ...string) ([]byte, error) {
			if !listDone {
				listDone = true

				return []byte("pool/fs@snap\t1024\n"), nil
			}

			return []byte("4096\n"), nil
		},
	}
	client := NewClient(runner, io.Discard)

	snapshots, err := client.ListSnapshots("", false, false)
	if err != nil {
		t.Fatalf("ListSnapshots() error = %v", err)
	}

	if err := client.DestroySnapshot("pool/fs@old", false, false); err != nil {
		t.Fatalf("DestroySnapshot() error = %v", err)
	}

	if got := snapshots[0].GetUsed(false); got != 4096 {
		t.Errorf("GetUsed() after destroy = %d, want 4096", got)
	}

	wantRun := []commandCall{{name: "zfs", args: []string{"destroy", "-d", "pool/fs@old"}}}
	if diff := deep.Equal(runner.calls[1:2], wantRun); diff != nil {
		t.Errorf("destroy command differs: %v", diff)
	}
}

func TestDestroySnapshotErrorAndDryRun(t *testing.T) {
	t.Parallel()

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		runner := &fakeRunner{err: errTestCommand}
		if err := NewClient(runner, io.Discard).DestroySnapshot("pool/fs@snap", false, false); err == nil {
			t.Fatal("DestroySnapshot() error = nil, want command error")
		}
	})

	t.Run("dry run", func(t *testing.T) {
		t.Parallel()

		runner := &fakeRunner{}

		client := NewClient(runner, io.Discard)
		if err := client.DestroySnapshot("pool/fs@snap", true, false); err != nil {
			t.Fatalf("DestroySnapshot() error = %v", err)
		}

		if len(runner.calls) != 0 {
			t.Errorf("Run calls = %d, want 0", len(runner.calls))
		}

		if !client.snapshotState.stale.Load() {
			t.Error("snapshot state was not invalidated")
		}
	})
}
