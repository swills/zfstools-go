package zfs

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"strconv"
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

			got, err := snapshot.GetUsed(t.Context(), false)
			if err != nil {
				t.Fatalf("GetUsed() error = %v", err)
			}

			if got != testCase.want {
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
		err     error
		wantErr error
		name    string
		output  string
	}{
		{name: "command error", err: errTestCommand, wantErr: errTestCommand},
		{name: "invalid size", output: "invalid\n", wantErr: strconv.ErrSyntax},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			runner := &fakeRunner{output: []byte(testCase.output), err: testCase.err}

			snapshot := Snapshot{Name: "pool/fs@snap", runner: runner, state: &snapshotState{}}

			got, err := snapshot.GetUsed(t.Context(), false)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("GetUsed() error = %v, want %v", err, testCase.wantErr)
			}

			if got != 0 {
				t.Errorf("GetUsed() = %d, want 0", got)
			}
		})
	}
}

func TestSnapshotGetUsedUsesCachedValueWithoutClient(t *testing.T) {
	t.Parallel()

	snapshot := Snapshot{Used: 1024}

	got, err := snapshot.GetUsed(t.Context(), false)
	if err != nil {
		t.Fatalf("GetUsed() error = %v", err)
	}

	if got != 1024 {
		t.Errorf("GetUsed() = %d, want 1024", got)
	}
}

func TestListSnapshots(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr   error
		name      string
		dataset   string
		output    string
		wantArgs  []string
		want      []Snapshot
		recursive bool
	}{
		{
			name:   "all",
			output: "tank/data@backup\t134217728\t1735794245\n",
			wantArgs: []string{
				"list", "-H", "-p", "-t", "snapshot", "-o", "name,used,creation", "-S", "creation",
			},
			want: []Snapshot{{Name: "tank/data@backup", Used: 134217728, Creation: 1735794245}},
		},
		{
			name:      "named recursive",
			dataset:   "tank",
			recursive: true,
			output:    "tank/data@backup1\t131072\t1735794245\n",
			wantArgs: []string{
				"list", "-r", "-H", "-p", "-t", "snapshot", "-o", "name,used,creation", "-S", "creation", "tank",
			},
			want: []Snapshot{{Name: "tank/data@backup1", Used: 131072, Creation: 1735794245}},
		},
		{
			name:    "named non-recursive",
			dataset: "tank",
			wantArgs: []string{
				"list", "-d", "1", "-H", "-p", "-t", "snapshot",
				"-o", "name,used,creation", "-S", "creation", "tank",
			},
			want: []Snapshot{},
		},
		{
			name: "malformed rows",
			output: "invalid\n" +
				"invalid@size\tnot-a-number\t1735794245\n" +
				"invalid@creation\t1\tnot-a-number\n",
			wantArgs: []string{
				"list", "-H", "-p", "-t", "snapshot", "-o", "name,used,creation", "-S", "creation",
			},
			wantErr: errInvalidSnapshotOutput,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			runner := &fakeRunner{output: []byte(testCase.output)}

			got, err := NewClient(runner, io.Discard).ListSnapshots(
				t.Context(), testCase.dataset, testCase.recursive, false,
			)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("ListSnapshots() error = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantErr != nil {
				if got != nil {
					t.Errorf("ListSnapshots() = %v, want nil result", got)
				}
			} else {
				publicGot := make([]Snapshot, len(got))
				for i := range got {
					publicGot[i] = Snapshot{Name: got[i].Name, Used: got[i].Used, Creation: got[i].Creation}
				}

				if diff := deep.Equal(publicGot, testCase.want); diff != nil {
					t.Errorf("snapshots differ: %v", diff)
				}
			}

			wantCall := []commandCall{{name: "zfs", args: testCase.wantArgs}}
			if diff := deep.Equal(runner.calls, wantCall); diff != nil {
				t.Errorf("command differs: %v", diff)
			}
		})
	}
}

func TestParseSnapshotsReaderError(t *testing.T) {
	t.Parallel()

	reader := &errorReader{data: []byte("tank/data@snap\t1\t1735794245\n")}

	got, err := parseSnapshots(reader, &fakeRunner{}, io.Discard, &snapshotState{})
	if !errors.Is(err, errTestReader) {
		t.Fatalf("parseSnapshots() error = %v, want reader error", err)
	}

	if got != nil {
		t.Fatalf("parseSnapshots() = %v, want nil result", got)
	}
}

func TestListSnapshotsCommandError(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{output: []byte("tank/data@snap\t1\t1735794245\n"), err: errTestCommand}

	got, err := NewClient(runner, io.Discard).ListSnapshots(t.Context(), "", false, false)
	if !errors.Is(err, errTestCommand) {
		t.Fatalf("ListSnapshots() error = %v, want command error", err)
	}

	if got != nil {
		t.Fatalf("ListSnapshots() = %v, want nil result", got)
	}
}

func TestCreateSnapshots(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr      error
		name         string
		database     string
		snapshotName string
		datasetNames []string
		wantCalls    []commandCall
		recursive    bool
		dryRun       bool
	}{
		{name: "empty snapshot name", datasetNames: []string{"pool/fs"}, wantErr: ErrEmptySnapshotName},
		{name: "no datasets", snapshotName: "snap", wantErr: ErrNoDatasets},
		{
			name: "snapshot separator in dataset", datasetNames: []string{"pool/fs@old"}, snapshotName: "snap",
			wantErr: ErrInvalidSnapshotName,
		},
		{
			name: "snapshot separator in name", datasetNames: []string{"pool/fs"}, snapshotName: "old@snap",
			wantErr: ErrInvalidSnapshotName,
		},
		{
			name: "single", datasetNames: []string{"pool/fs"}, snapshotName: "snap",
			wantCalls: []commandCall{{name: "zfs", args: []string{"snapshot", "pool/fs@snap"}}},
		},
		{
			name: "direct argument with shell syntax", datasetNames: []string{"pool/fs; touch /tmp/pwn"},
			snapshotName: "snap",
			wantCalls: []commandCall{{
				name: "zfs", args: []string{"snapshot", "pool/fs; touch /tmp/pwn@snap"},
			}},
		},
		{
			name: "multiple recursive", datasetNames: []string{"pool/fs1", "pool/fs2"}, snapshotName: "snap",
			recursive: true,
			wantCalls: []commandCall{{
				name: "zfs", args: []string{"snapshot", "-r", "pool/fs1@snap", "pool/fs2@snap"},
			}},
		},
		{
			name: "mysql", datasetNames: []string{"pool/fs"}, snapshotName: "snap", database: "mysql",
			wantCalls: []commandCall{{name: "mysql", args: []string{
				"-e", "FLUSH LOGS; FLUSH TABLES WITH READ LOCK; " +
					"SYSTEM zfs snapshot pool/fs@snap; UNLOCK TABLES;",
			}}},
		},
		{
			name: "mysql shell syntax", datasetNames: []string{"pool/fs; touch /tmp/pwn"},
			snapshotName: "snap", database: "mysql",
			wantCalls: []commandCall{{name: "mysql", args: []string{
				"-e", "FLUSH LOGS; FLUSH TABLES WITH READ LOCK; " +
					"SYSTEM zfs snapshot 'pool/fs; touch /tmp/pwn@snap'; UNLOCK TABLES;",
			}}},
		},
		{
			name: "postgresql", datasetNames: []string{"pool/fs"}, snapshotName: "snap", database: "postgresql",
			wantCalls: []commandCall{
				{name: "psql", args: []string{"-c", "SELECT PG_START_BACKUP('zfs-auto-snapshot');", "postgres"}},
				{name: "zfs", args: []string{"snapshot", "pool/fs@snap"}},
				{name: "psql", args: []string{"-c", "SELECT PG_STOP_BACKUP();", "postgres"}},
			},
		},
		{name: "dry run", datasetNames: []string{"pool/fs"}, snapshotName: "snap", dryRun: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			runner := &fakeRunner{}
			client := NewClient(runner, io.Discard)

			err := client.CreateSnapshots(
				t.Context(),
				testCase.datasetNames, testCase.snapshotName, testCase.recursive, testCase.database,
				testCase.dryRun, false, false,
			)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("CreateSnapshots() error = %v, want %v", err, testCase.wantErr)
			}

			if diff := deep.Equal(runner.calls, testCase.wantCalls); diff != nil {
				t.Errorf("commands differ: %v", diff)
			}
		})
	}
}

func TestCreateSnapshotsCommandError(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{err: errTestCommand}
	if err := NewClient(runner, io.Discard).CreateSnapshots(
		t.Context(),
		[]string{"pool/fs"}, "snap", false, "", false, false, false,
	); err == nil {
		t.Fatal("CreateSnapshots() error = nil, want command error")
	}
}

func TestCreateDatabaseSnapshotsDryRunOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		db   string
		want []string
	}{
		{
			name: "mysql",
			db:   "mysql",
			want: []string{"mysql -e", "SYSTEM zfs snapshot pool/database@snap"},
		},
		{
			name: "postgresql",
			db:   "postgresql",
			want: []string{"psql -c", "PG_START_BACKUP", "zfs snapshot pool/database@snap", "PG_STOP_BACKUP"},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			runner := &fakeRunner{}
			output := &bytes.Buffer{}
			client := NewClient(runner, output)

			err := client.CreateSnapshots(
				t.Context(),
				[]string{"pool/database"}, "snap", false, testCase.db, true, true, false,
			)
			if err != nil {
				t.Fatalf("CreateSnapshots() error = %v", err)
			}

			for _, want := range testCase.want {
				if !bytes.Contains(output.Bytes(), []byte(want)) {
					t.Errorf("CreateSnapshots() output = %q, want substring %q", output.String(), want)
				}
			}

			if len(runner.calls) != 0 {
				t.Errorf("Run calls = %d, want 0", len(runner.calls))
			}
		})
	}
}

func TestShellQuoteEmptyString(t *testing.T) {
	t.Parallel()

	if got := shellQuote(""); got != "''" {
		t.Errorf("shellQuote() = %q, want empty shell string", got)
	}
}

func TestCreatePostgreSQLSnapshotsAttemptsStopAfterErrors(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{err: errTestCommand}

	err := NewClient(runner, io.Discard).CreateSnapshots(
		t.Context(),
		[]string{"pool/postgres"}, "snap", false, "postgresql", false, false, false,
	)
	if !errors.Is(err, errTestCommand) {
		t.Fatalf("CreateSnapshots() error = %v, want command error", err)
	}

	want := []commandCall{
		{name: "psql", args: []string{"-c", "SELECT PG_START_BACKUP('zfs-auto-snapshot');", "postgres"}},
		{name: "zfs", args: []string{"snapshot", "pool/postgres@snap"}},
		{name: "psql", args: []string{"-c", "SELECT PG_STOP_BACKUP();", "postgres"}},
	}
	if diff := deep.Equal(runner.calls, want); diff != nil {
		t.Errorf("commands differ: %v", diff)
	}
}

func TestCreateManySnapshots(t *testing.T) {
	t.Parallel()

	t.Run("multi-snapshot", func(t *testing.T) {
		t.Parallel()

		runner := &fakeRunner{runFunc: func(name string, _ ...string) ([]byte, error) {
			if name == "zpool" {
				return []byte("pool\tfeature@bookmarks\tenabled\n"), nil
			}

			return []byte("123456\n"), nil
		}}
		client := NewClient(runner, io.Discard)

		created, err := client.CreateManySnapshots(t.Context(), "auto",
			[]Dataset{{Name: "pool/fs1"}, {Name: "pool/fs2"}}, false, false, false, false, false)
		if err != nil {
			t.Fatalf("createManySnapshots() error = %v", err)
		}

		want := []commandCall{
			{name: "zpool", args: []string{
				"get", "-H", "-p", "-o", "name,property,value", "feature@bookmarks",
			}},
			{name: "getconf", args: []string{"ARG_MAX"}},
			{name: "zfs", args: []string{"snapshot", "pool/fs1@auto", "pool/fs2@auto"}},
		}

		if diff := deep.Equal(runner.calls, want); diff != nil {
			t.Errorf("commands differ: %v", diff)
		}

		if !slices.Equal(created, []string{"pool/fs1", "pool/fs2"}) {
			t.Errorf("created datasets = %v, want both datasets", created)
		}
	})

	t.Run("single snapshots", func(t *testing.T) {
		t.Parallel()

		runner := &fakeRunner{}
		client := NewClient(runner, io.Discard)

		created, err := client.CreateManySnapshots(t.Context(), "auto",
			[]Dataset{{Name: "pool/fs1"}, {Name: "pool/fs2"}}, false, false, false, false, false)
		if err != nil {
			t.Fatalf("createManySnapshots() error = %v", err)
		}

		want := []commandCall{
			{name: "zpool", args: []string{
				"get", "-H", "-p", "-o", "name,property,value", "feature@bookmarks",
			}},
			{name: "zfs", args: []string{"snapshot", "pool/fs1@auto"}},
			{name: "zfs", args: []string{"snapshot", "pool/fs2@auto"}},
		}
		if diff := deep.Equal(runner.calls, want); diff != nil {
			t.Errorf("commands differ: %v", diff)
		}

		if !slices.Equal(created, []string{"pool/fs1", "pool/fs2"}) {
			t.Errorf("created datasets = %v, want both datasets", created)
		}
	})

	t.Run("one command fails", func(t *testing.T) {
		t.Parallel()

		runner := &fakeRunner{runFunc: func(name string, args ...string) ([]byte, error) {
			if name == "zfs" && args[len(args)-1] == "pool/fs2@auto" {
				return nil, errTestCommand
			}

			return nil, nil
		}}
		client := NewClient(runner, io.Discard)

		created, err := client.CreateManySnapshots(t.Context(), "auto",
			[]Dataset{{Name: "pool/fs1"}, {Name: "pool/fs2"}}, false, false, false, false, false)
		if !errors.Is(err, ErrOneSnapshotOfManyErrored) {
			t.Fatalf("createManySnapshots() error = %v, want %v", err, ErrOneSnapshotOfManyErrored)
		}

		if !slices.Equal(created, []string{"pool/fs1"}) {
			t.Errorf("created datasets = %v, want pool/fs1", created)
		}
	})
}

func TestCreateManySnapshotsPooledFailureConfirmsNoDatasets(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{runFunc: func(name string, _ ...string) ([]byte, error) {
		switch name {
		case "zpool":
			return []byte("pool\tfeature@bookmarks\tenabled\n"), nil
		case "getconf":
			return []byte("123456\n"), nil
		case "zfs":
			return nil, errTestCommand
		default:
			return nil, nil
		}
	}}
	client := NewClient(runner, io.Discard)

	created, err := client.CreateManySnapshots(t.Context(), "auto",
		[]Dataset{{Name: "pool/fs1"}, {Name: "pool/fs2"}}, false, false, false, false, false)
	if !errors.Is(err, ErrOneSnapshotOfManyErrored) {
		t.Fatalf("CreateManySnapshots() error = %v, want pooled command error", err)
	}

	if len(created) != 0 {
		t.Errorf("created datasets = %v, want none", created)
	}
}

func TestCreateManySnapshotsDatabaseDataset(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	client := NewClient(runner, io.Discard)

	created, err := client.CreateManySnapshots(
		t.Context(),
		"auto", []Dataset{{Name: "pool/mysql", DB: "mysql"}}, false, false, false, false, false,
	)
	if err != nil {
		t.Fatalf("CreateManySnapshots() error = %v", err)
	}

	want := []commandCall{{name: "mysql", args: []string{
		"-e", "FLUSH LOGS; FLUSH TABLES WITH READ LOCK; " +
			"SYSTEM zfs snapshot pool/mysql@auto; UNLOCK TABLES;",
	}}}
	if diff := deep.Equal(runner.calls, want); diff != nil {
		t.Errorf("commands differ: %v", diff)
	}

	if !slices.Equal(created, []string{"pool/mysql"}) {
		t.Errorf("created datasets = %v, want pool/mysql", created)
	}
}

func TestCreateManySnapshotsMixedDatasetsContinueAfterError(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{runFunc: func(name string, _ ...string) ([]byte, error) {
		if name == "mysql" {
			return nil, errTestCommand
		}

		return nil, nil
	}}
	client := NewClient(runner, io.Discard)

	created, err := client.CreateManySnapshots(t.Context(), "auto", []Dataset{
		{Name: "pool/mysql", DB: "mysql"},
		{Name: "pool/files"},
	}, false, false, false, false, false)
	if !errors.Is(err, ErrOneSnapshotOfManyErrored) {
		t.Fatalf("CreateManySnapshots() error = %v, want %v", err, ErrOneSnapshotOfManyErrored)
	}

	if !slices.Equal(created, []string{"pool/files"}) {
		t.Errorf("created datasets = %v, want pool/files", created)
	}

	if len(runner.calls) != 3 {
		t.Fatalf("command count = %d, want 3", len(runner.calls))
	}

	if got := runner.calls[2]; deep.Equal(got, commandCall{
		name: "zfs", args: []string{"snapshot", "pool/files@auto"},
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
		{name: "failure", fail: "pool/fs2@auto", wantErr: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			runner := &fakeRunner{runFunc: func(name string, args ...string) ([]byte, error) {
				if name == "zfs" && args[len(args)-1] == testCase.fail {
					return nil, errTestCommand
				}

				return nil, nil
			}}
			client := NewClient(runner, io.Discard)

			created, err := client.CreateManySnapshots(t.Context(), "auto", []Dataset{
				{Name: "pool/fs1"}, {Name: "pool/fs2"}, {Name: "pool/fs3"},
			}, false, false, false, false, true)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("CreateManySnapshots() error = %v, wantErr %v", err, testCase.wantErr)
			}

			wantCreated := 3
			if testCase.wantErr {
				wantCreated = 2
			}

			if len(created) != wantCreated {
				t.Errorf("created dataset count = %d, want %d", len(created), wantCreated)
			}

			targets := make([]string, 0, len(runner.calls))
			for _, call := range runner.calls {
				if call.name == "zpool" {
					continue
				}

				if call.name != "zfs" || len(call.args) != 2 || call.args[0] != "snapshot" {
					t.Fatalf("unexpected command: %#v", call)
				}

				targets = append(targets, call.args[1])
			}

			slices.Sort(targets)

			want := []string{
				"pool/fs1@auto",
				"pool/fs2@auto",
				"pool/fs3@auto",
			}
			if diff := deep.Equal(targets, want); diff != nil {
				t.Errorf("commands differ: %v", diff)
			}
		})
	}
}

func TestCreateManySnapshotsMinimumChunkSize(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{runFunc: func(name string, _ ...string) ([]byte, error) {
		if name == "zpool" {
			return []byte("pool\tfeature@bookmarks\tenabled\n"), nil
		}

		return []byte("1\n"), nil
	}}
	client := NewClient(runner, io.Discard)

	created, err := client.CreateManySnapshots(t.Context(), "auto", []Dataset{
		{Name: "pool/fs1"}, {Name: "pool/fs2"},
	}, false, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateManySnapshots() error = %v", err)
	}

	want := []commandCall{
		{name: "zpool", args: []string{
			"get", "-H", "-p", "-o", "name,property,value", "feature@bookmarks",
		}},
		{name: "getconf", args: []string{"ARG_MAX"}},
		{name: "zfs", args: []string{"snapshot", "pool/fs1@auto"}},
		{name: "zfs", args: []string{"snapshot", "pool/fs2@auto"}},
	}
	if diff := deep.Equal(runner.calls, want); diff != nil {
		t.Errorf("commands differ: %v", diff)
	}

	if !slices.Equal(created, []string{"pool/fs1", "pool/fs2"}) {
		t.Errorf("created datasets = %v, want both datasets", created)
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
			name: "snapshot separator in name", snapshot: "old@auto",
			datasets: []Dataset{{Name: "pool/fs"}}, want: ErrInvalidSnapshotName,
		},
		{
			name: "snapshot in dataset", snapshot: "auto",
			datasets: []Dataset{{Name: "pool/fs@old"}}, want: ErrInvalidSnapshotName,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			client := NewClient(&fakeRunner{}, io.Discard)

			_, err := client.CreateManySnapshots(t.Context(), testCase.snapshot,
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
			if got := NewClient(runner, io.Discard).getArgMax(t.Context()); got != testCase.want {
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

				return []byte("pool/fs@snap\t1024\t1735794245\n"), nil
			}

			return []byte("4096\n"), nil
		},
	}
	client := NewClient(runner, io.Discard)

	snapshots, err := client.ListSnapshots(t.Context(), "", false, false)
	if err != nil {
		t.Fatalf("ListSnapshots() error = %v", err)
	}

	if destroyErr := client.DestroySnapshot(t.Context(), "pool/fs@old", false, false); destroyErr != nil {
		t.Fatalf("DestroySnapshot() error = %v", destroyErr)
	}

	got, err := snapshots[0].GetUsed(t.Context(), false)
	if err != nil {
		t.Fatalf("GetUsed() error = %v", err)
	}

	if got != 4096 {
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

		client := NewClient(runner, io.Discard)
		if err := client.DestroySnapshot(
			t.Context(), "pool/fs@snap", false, false,
		); err == nil {
			t.Fatal("DestroySnapshot() error = nil, want command error")
		}

		if client.snapshotState.stale.Load() {
			t.Error("failed destroy invalidated snapshot state")
		}
	})

	t.Run("dry run", func(t *testing.T) {
		t.Parallel()

		runner := &fakeRunner{}

		client := NewClient(runner, io.Discard)
		if err := client.DestroySnapshot(t.Context(), "pool/fs@snap", true, false); err != nil {
			t.Fatalf("DestroySnapshot() error = %v", err)
		}

		if len(runner.calls) != 0 {
			t.Errorf("Run calls = %d, want 0", len(runner.calls))
		}

		if client.snapshotState.stale.Load() {
			t.Error("dry run invalidated snapshot state")
		}
	})
}
