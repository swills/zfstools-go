package zfstools

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-test/deep"

	"zfstools-go/internal/config"
	"zfstools-go/internal/zfs"
)

type fakeRunner struct {
	runFunc func(string, ...string) ([]byte, error)
	output  []byte
	name    string
	args    []string
	calls   []commandCall
	mu      sync.Mutex
}

type commandCall struct {
	name string
	args []string
}

var errTestCommand = errors.New("test command failed")

func (runner *fakeRunner) Run(name string, args ...string) ([]byte, error) {
	runner.mu.Lock()
	runner.name = name

	runner.args = append([]string(nil), args...)
	runner.calls = append(runner.calls, commandCall{name: name, args: append([]string(nil), args...)})
	runFunc := runner.runFunc
	output := runner.output
	runner.mu.Unlock()

	if runFunc != nil {
		return runFunc(name, args...)
	}

	return output, nil
}

func TestDoNewSnapshots(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	tools := New(zfs.NewClient(runner, io.Discard), io.Discard)
	cfg := config.Config{
		Interval:  "frequent",
		Timestamp: time.Date(2025, 1, 2, 3, 4, 0, 0, time.UTC),
	}

	datasets := map[string][]zfs.Dataset{
		"single":    {{Name: "pool/fs1"}},
		"recursive": {{Name: "pool/fs2"}},
	}
	if err := tools.DoNewSnapshots(cfg, datasets); err != nil {
		t.Fatalf("DoNewSnapshots() error = %v", err)
	}

	want := []commandCall{
		{name: "zpool", args: []string{
			"get", "-H", "-p", "-o", "name,property,value", "feature@bookmarks",
		}},
		{name: "zfs", args: []string{"snapshot", "pool/fs1@zfs-auto-snap_frequent-2025-01-02-03h04"}},
		{name: "zfs", args: []string{
			"snapshot", "-r", "pool/fs2@zfs-auto-snap_frequent-2025-01-02-03h04",
		}},
	}
	if diff := deep.Equal(runner.calls, want); diff != nil {
		t.Errorf("commands differ: %v", diff)
	}
}

func TestDoNewSnapshotsContinuesAfterError(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{runFunc: func(name string, args ...string) ([]byte, error) {
		if name == "zfs" && args[len(args)-1] == "pool/fs1@zfs-auto-snap_frequent-2025-01-02-03h04" {
			return nil, errTestCommand
		}

		return nil, nil
	}}
	tools := New(zfs.NewClient(runner, io.Discard), io.Discard)
	cfg := config.Config{
		Interval: "frequent", Timestamp: time.Date(2025, 1, 2, 3, 4, 0, 0, time.UTC),
	}
	datasets := map[string][]zfs.Dataset{
		"single":    {{Name: "pool/fs1"}},
		"recursive": {{Name: "pool/fs2"}},
	}

	err := tools.DoNewSnapshots(cfg, datasets)
	if !errors.Is(err, zfs.ErrOneSnapshotOfManyErrored) {
		t.Fatalf("DoNewSnapshots() error = %v, want snapshot error", err)
	}

	if !errors.Is(err, errTestCommand) {
		t.Errorf("DoNewSnapshots() error = %v, want underlying command error", err)
	}

	if got := len(runner.calls); got != 3 {
		t.Errorf("command count = %d, want feature check and both snapshots", got)
	}
}

func TestGroupSnapshotsIntoDatasets(t *testing.T) {
	t.Parallel()

	type args struct {
		snaps    []zfs.Snapshot
		datasets []zfs.Dataset
	}

	tests := []struct {
		want map[string][]zfs.Snapshot
		name string
		args args
	}{
		{
			name: "simple",
			args: args{
				snaps: []zfs.Snapshot{
					{Name: "pool/home@zfs-auto-snap_hourly-2025-01-01-01h00"},
					{Name: "pool/data@zfs-auto-snap_hourly-2025-01-01-01h00"},
				},
				datasets: []zfs.Dataset{
					{Name: "pool/home"},
					{Name: "pool/data"},
				},
			},
			want: map[string][]zfs.Snapshot{
				"pool/home": {
					{
						Name: "pool/home@zfs-auto-snap_hourly-2025-01-01-01h00",
					},
				},
				"pool/data": {
					{
						Name: "pool/data@zfs-auto-snap_hourly-2025-01-01-01h00",
					},
				},
			},
		},
		{
			name: "Groups snapshots into their datasets",
			args: args{
				snaps: []zfs.Snapshot{
					{Name: "tank@1"},
					{Name: "tank@2"},
					{Name: "tank/a@1"},
					{Name: "tank/a@2"},
					{Name: "tank/a/1@1"},
					{Name: "tank/a/2@1"},
					{Name: "tank/b@1"},
					{Name: "tank/c@1"},
					{Name: "tank/d@1"},
					{Name: "tank/d/1@2"},
				},
				datasets: []zfs.Dataset{
					{Name: "tank"},
					{Name: "tank/a"},
					{Name: "tank/a/1"},
					{Name: "tank/a/2"},
					{Name: "tank/b"},
					{Name: "tank/c"},
					{Name: "tank/d"},
					{Name: "tank/d/1"},
				},
			},
			want: map[string][]zfs.Snapshot{
				"tank": {
					{
						Name: "tank@1",
					},
					{
						Name: "tank@2",
					},
				},
				"tank/a": {
					{
						Name: "tank/a@1",
					},
					{
						Name: "tank/a@2",
					},
				},
				"tank/a/1": {
					{
						Name: "tank/a/1@1",
					},
				},
				"tank/a/2": {
					{
						Name: "tank/a/2@1",
					},
				},
				"tank/b": {
					{
						Name: "tank/b@1",
					},
				},
				"tank/c": {
					{
						Name: "tank/c@1",
					},
				},
				"tank/d": {
					{
						Name: "tank/d@1",
					},
				},
				"tank/d/1": {
					{
						Name: "tank/d/1@2",
					},
				},
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := GroupSnapshotsIntoDatasets(testCase.args.snaps, testCase.args.datasets)

			diff := deep.Equal(got, testCase.want)
			if diff != nil {
				t.Errorf("compare failed: %#v", diff)
			}
		})
	}
}

func TestFindEligibleDatasetsMountEligibility(t *testing.T) {
	t.Parallel()

	type args struct {
		pool string
		cfg  config.Config
	}

	tests := []struct {
		want        map[string][]zfs.Dataset
		name        string
		mockCmdFunc string
		args        args
	}{
		{
			name:        "noExistingSnapshotsOneDataset",
			mockCmdFunc: "TestFindEligibleDatasets_noExistingSnapshotsOneDataset",
			args: args{
				cfg:  config.Config{Interval: "frequent"},
				pool: "",
			},
			want: map[string][]zfs.Dataset{
				"single": nil,
				"recursive": {
					{
						Name: "tank/fs1",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "true",
							"mounted":               "yes",
						},
						DB: "",
					},
				},
				"included": {
					{
						Name: "tank/fs1",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "true",
							"mounted":               "yes",
						},
						DB: "",
					},
				},
				"excluded": nil,
			},
		},
		{
			name:        "noExistingSnapshotsTwoDatasetsOneUnmounted",
			mockCmdFunc: "TestFindEligibleDatasets_noExistingSnapshotsTwoDatasetsOneUnmounted",
			args: args{
				cfg:  config.Config{Interval: "frequent"},
				pool: "",
			},
			want: map[string][]zfs.Dataset{
				"single": nil,
				"recursive": {
					{
						Name: "tank/fs1",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "true",
							"mounted":               "yes",
						},
						DB: "",
					},
				},
				"included": {
					{
						Name: "tank/fs1",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "true",
							"mounted":               "yes",
						},
						DB: "",
					},
				},
				"excluded": {
					{
						Name: "tank/fs2",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "true",
							"mounted":               "no",
						},
						DB: "",
					},
				},
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assertFindEligibleDatasets(t, testCase.mockCmdFunc, testCase.args.cfg, testCase.args.pool, testCase.want)
		})
	}
}

func TestFindEligibleDatasetsPropertyEligibility(t *testing.T) {
	t.Parallel()

	type args struct {
		pool string
		cfg  config.Config
	}

	tests := []struct {
		want        map[string][]zfs.Dataset
		name        string
		mockCmdFunc string
		args        args
	}{
		{
			name:        "manyFS",
			mockCmdFunc: "TestFindEligibleDatasets_alreadyFound",
			args: args{
				cfg:  config.Config{Interval: "frequent"},
				pool: "",
			},
			want: map[string][]zfs.Dataset{
				"single": nil,
				"recursive": {
					{
						Name: "tank/fs2",
						Properties: map[string]string{
							"type":                           "filesystem",
							"com.sun:auto-snapshot:frequent": "true",
							"com.sun:auto-snapshot":          "true",
							"mounted":                        "yes",
						},
						DB: "",
					},
					{
						Name: "tank/fs1",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "true",
							"mounted":               "yes",
						},
						DB: "",
					},
				},
				"included": {
					{
						Name: "tank/fs2",
						Properties: map[string]string{
							"type":                           "filesystem",
							"com.sun:auto-snapshot:frequent": "true",
							"com.sun:auto-snapshot":          "true",
							"mounted":                        "yes",
						},
						DB: "",
					},
					{
						Name: "tank/fs1",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "true",
							"mounted":               "yes",
						},
						DB: "",
					},
				},
				"excluded": nil,
			},
		},
		{
			name:        "onlyFreq",
			mockCmdFunc: "TestFindEligibleDatasets_onlyFreq",
			args: args{
				cfg:  config.Config{Interval: "frequent"},
				pool: "",
			},
			want: map[string][]zfs.Dataset{
				"single": nil,
				"recursive": {
					{
						Name: "tank/fs2",
						Properties: map[string]string{
							"type":                           "filesystem",
							"com.sun:auto-snapshot:frequent": "true",
							"com.sun:auto-snapshot":          "true",
							"mounted":                        "yes",
						},
						DB: "",
					},
					{
						Name: "tank/fs3",
						Properties: map[string]string{
							"type":                           "filesystem",
							"com.sun:auto-snapshot:frequent": "true",
							"mounted":                        "yes",
						},
						DB: "",
					},
					{
						Name: "tank/fs1",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "true",
							"mounted":               "yes",
						},
						DB: "",
					},
				},
				"included": {
					{
						Name: "tank/fs2",
						Properties: map[string]string{
							"type":                           "filesystem",
							"com.sun:auto-snapshot:frequent": "true",
							"com.sun:auto-snapshot":          "true",
							"mounted":                        "yes",
						},
						DB: "",
					},
					{
						Name: "tank/fs3",
						Properties: map[string]string{
							"type":                           "filesystem",
							"com.sun:auto-snapshot:frequent": "true",
							"mounted":                        "yes",
						},
						DB: "",
					},
					{
						Name: "tank/fs1",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "true",
							"mounted":               "yes",
						},
						DB: "",
					},
				},
				"excluded": nil,
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assertFindEligibleDatasets(t, testCase.mockCmdFunc, testCase.args.cfg, testCase.args.pool, testCase.want)
		})
	}
}

func TestFindEligibleDatasetsPoolHierarchy(t *testing.T) {
	t.Parallel()

	type args struct {
		pool string
		cfg  config.Config
	}

	tests := []struct {
		want        map[string][]zfs.Dataset
		name        string
		mockCmdFunc string
		args        args
	}{
		{
			name:        "manyDatasets",
			mockCmdFunc: "TestFindEligibleDatasets_manyDatasets",
			args: args{
				cfg:  config.Config{Interval: "frequent"},
				pool: "tank",
			},
			want: map[string][]zfs.Dataset{
				"single": {
					{
						Name: "tank/moredata",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "true",
							"mounted":               "yes",
						},
						DB: "",
					},
				},
				"recursive": {
					{
						Name: "tank/ROOT/default",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "true",
							"mounted":               "yes",
						},
						DB: "",
					},
					{
						Name: "tank/poudriere/ports/default",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "true",
							"mounted":               "yes",
						},
						DB: "",
					},
					{
						Name: "tank/usr/home",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "true",
							"mounted":               "yes",
						},
						DB: "",
					},
					{
						Name: "tank/moredata/3",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "true",
							"mounted":               "yes",
						},
						DB: "",
					},
				},
				"included": {
					{
						Name: "tank/ROOT/default",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "true",
							"mounted":               "yes",
						},
						DB: "",
					},
					{
						Name: "tank/poudriere/ports/default",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "true",
							"mounted":               "yes",
						},
						DB: "",
					},
					{
						Name: "tank/usr/home",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "true",
							"mounted":               "yes",
						},
						DB: "",
					},
					{
						Name: "tank/moredata",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "true",
							"mounted":               "yes",
						},
						DB: "",
					},
					{
						Name: "tank/moredata/3",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "true",
							"mounted":               "yes",
						},
						DB: "",
					},
				},
				"excluded": {
					{
						Name: "tank/poudriere",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "false",
							"mounted":               "yes",
						},
						DB: "",
					},
					{
						Name: "tank/poudriere/ccache",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "false",
							"mounted":               "yes",
						},
						DB: "",
					},
					{
						Name: "tank/poudriere/data",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "false",
							"mounted":               "yes",
						},
						DB: "",
					},
					{
						Name: "tank/poudriere/data/cache",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "false",
							"mounted":               "yes",
						},
						DB: "",
					},
					{
						Name: "tank/poudriere/data/logs",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "false",
							"mounted":               "yes",
						},
						DB: "",
					},
					{
						Name: "tank/poudriere/data/packages",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "false",
							"mounted":               "yes",
						},
						DB: "",
					},
					{
						Name: "tank/poudriere/data/wrkdirs",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "false",
							"mounted":               "yes",
						},
						DB: "",
					},
					{
						Name: "tank/poudriere/distfiles",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "false",
							"mounted":               "yes",
						},
						DB: "",
					},
					{
						Name: "tank/poudriere/jails",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "false",
							"mounted":               "yes",
						},
						DB: "",
					},
					{
						Name: "tank/poudriere/jails/head-amd64",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "false",
							"mounted":               "yes",
						},
						DB: "",
					},
					{
						Name: "tank/poudriere/ports",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "false",
							"mounted":               "yes",
						},
						DB: "",
					},
					{
						Name: "tank/moredata/2",
						Properties: map[string]string{
							"type":                  "filesystem",
							"com.sun:auto-snapshot": "false",
							"mounted":               "yes",
						},
						DB: "",
					},
				},
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assertFindEligibleDatasets(t, testCase.mockCmdFunc, testCase.args.cfg, testCase.args.pool, testCase.want)
		})
	}
}

func assertFindEligibleDatasets(
	t *testing.T,
	outputName string,
	cfg config.Config,
	pool string,
	want map[string][]zfs.Dataset,
) {
	t.Helper()

	runner := &fakeRunner{output: findEligibleDatasetsOutput(outputName)}
	client := zfs.NewClient(runner, io.Discard)
	got := New(client, io.Discard).FindEligibleDatasets(cfg, pool)

	diff := deep.Equal(got, want)
	if diff != nil {
		t.Errorf("compare failed: %#v", diff)
	}

	wantArgs := []string{
		"list", "-H", "-t", "filesystem,volume", "-o",
		"name,type,com.sun:auto-snapshot:frequent,com.sun:auto-snapshot,mounted", "-s", "name",
	}
	if pool != "" {
		wantArgs = append(wantArgs, "-r", pool)
	}

	if runner.name != "zfs" || deep.Equal(runner.args, wantArgs) != nil {
		t.Errorf("command = %s %v, want zfs %v", runner.name, runner.args, wantArgs)
	}
}

func Test_findRecursiveDatasetsIncludedRoots(t *testing.T) {
	t.Parallel()

	type args struct {
		datasets map[string][]zfs.Dataset
	}

	tests := []struct {
		args args
		want map[string][]zfs.Dataset
		name string
	}{
		{
			name: "considers all included as recursive",
			args: args{
				datasets: map[string][]zfs.Dataset{
					"included": {
						{
							Name: "tank",
						},
						{
							Name: "tank/a",
						},
						{
							Name: "tank/a/1",
						},
						{
							Name: "tank/b",
						},
					},
				},
			},
			want: map[string][]zfs.Dataset{
				"single": nil,
				"recursive": {
					{
						Name:       "tank",
						Properties: nil,
						DB:         "",
					},
				},
				"included": {
					{
						Name: "tank",
					},
					{
						Name: "tank/a",
					},
					{
						Name: "tank/a/1",
					},
					{
						Name: "tank/b",
					},
				},
				"excluded": nil,
			},
		},
		{
			name: "considers all multiple parent datasets as recursive",
			args: args{
				datasets: map[string][]zfs.Dataset{
					"included": {
						{
							Name: "tank",
						},
						{
							Name: "tank/a",
						},
						{
							Name: "tank/a/1",
						},
						{
							Name: "tank/b",
						},
						{
							Name: "rpool",
						},
						{
							Name: "rpool/a",
						},
						{
							Name: "rpool/b",
						},
						{
							Name: "zpool",
						},
						{
							Name: "zpool/a",
						},
						{
							Name: "zpool/b",
						},
					},
					"excluded": {},
				},
			},
			want: map[string][]zfs.Dataset{
				"single": nil,
				"recursive": {
					{
						Name:       "tank",
						Properties: nil,
						DB:         "",
					},
					{
						Name:       "rpool",
						Properties: nil,
						DB:         "",
					},
					{
						Name:       "zpool",
						Properties: nil,
						DB:         "",
					},
				},
				"included": {
					{
						Name: "tank",
					},
					{
						Name: "tank/a",
					},
					{
						Name: "tank/a/1",
					},
					{
						Name: "tank/b",
					},
					{
						Name: "rpool",
					},
					{
						Name: "rpool/a",
					},
					{
						Name: "rpool/b",
					},
					{
						Name: "zpool",
					},
					{
						Name: "zpool/a",
					},
					{
						Name: "zpool/b",
					},
				},
				"excluded": {},
			},
		},
		{
			name: "considers all excluded as empty",
			args: args{
				datasets: map[string][]zfs.Dataset{
					"included": {},
					"excluded": {
						{
							Name: "tank",
						},
						{
							Name: "tank/a",
						},
						{
							Name: "tank/a/1",
						},
						{
							Name: "tank/b",
						},
					},
				},
			},
			want: map[string][]zfs.Dataset{
				"single":    nil,
				"recursive": nil,
				"included":  {},
				"excluded": {
					{
						Name: "tank",
					},
					{
						Name: "tank/a",
					},
					{
						Name: "tank/a/1",
					},
					{
						Name: "tank/b",
					},
				},
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assertRecursiveDatasets(t, testCase.args.datasets, testCase.want)
		})
	}
}

func Test_findRecursiveDatasetsExclusionBoundaries(t *testing.T) {
	t.Parallel()

	type args struct {
		datasets map[string][]zfs.Dataset
	}

	tests := []struct {
		args args
		want map[string][]zfs.Dataset
		name string
	}{
		{
			name: "considers first level excluded",
			args: args{
				datasets: map[string][]zfs.Dataset{
					"included": {
						{
							Name: "tank",
						},
						{
							Name: "tank/a",
						},
						{
							Name: "tank/a/1",
						},
					},
					"excluded": {
						{
							Name: "rpool",
						},
						{
							Name: "rpool/a",
						},
					},
				},
			},
			want: map[string][]zfs.Dataset{
				"single": nil,
				"recursive": {
					{
						Name: "tank",
					},
				},
				"included": {
					{
						Name: "tank",
					},
					{
						Name: "tank/a",
					},
					{
						Name: "tank/a/1",
					},
				},
				"excluded": {
					{
						Name: "rpool",
					},
					{
						Name: "rpool/a",
					},
				},
			},
		},
		{
			name: "considers second level excluded",
			args: args{
				datasets: map[string][]zfs.Dataset{
					"included": {
						{
							Name: "tank",
						},
						{
							Name: "tank/a",
						},
						{
							Name: "tank/a/1",
						},
					},
					"excluded": {
						{
							Name: "tank/b",
						},
					},
				},
			},
			want: map[string][]zfs.Dataset{
				"single": {
					{
						Name: "tank",
					},
				},
				"recursive": {
					{
						Name: "tank/a",
					},
				},
				"included": {
					{
						Name: "tank",
					},
					{
						Name: "tank/a",
					},
					{
						Name: "tank/a/1",
					},
				},
				"excluded": {
					{
						Name: "tank/b",
					},
				},
			},
		},
		{
			name: "considers third level excluded",
			args: args{
				datasets: map[string][]zfs.Dataset{
					"included": {
						{
							Name: "tank",
						},
						{
							Name: "tank/a",
						},
						{
							Name: "tank/a/1",
						},
						{
							Name: "tank/a/2",
						},
						{
							Name: "tank/b",
						},
						{
							Name: "tank/b/1",
						},
						{
							Name: "tank/b/2",
						},
					},
					"excluded": {
						{
							Name: "tank/c",
						},
					},
				},
			},
			want: map[string][]zfs.Dataset{
				"single": {
					{
						Name: "tank",
					},
				},
				"recursive": {
					{
						Name: "tank/a",
					},
					{
						Name: "tank/b",
					},
				},
				"included": {
					{
						Name: "tank",
					},
					{
						Name: "tank/a",
					},
					{
						Name: "tank/a/1",
					},
					{
						Name: "tank/a/2",
					},
					{
						Name: "tank/b",
					},
					{
						Name: "tank/b/1",
					},
					{
						Name: "tank/b/2",
					},
				},
				"excluded": {
					{
						Name: "tank/c",
					},
				},
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assertRecursiveDatasets(t, testCase.args.datasets, testCase.want)
		})
	}
}

func Test_findRecursiveDatasetsDatabasePropagation(t *testing.T) {
	t.Parallel()

	type args struct {
		datasets map[string][]zfs.Dataset
	}

	tests := []struct {
		args args
		want map[string][]zfs.Dataset
		name string
	}{
		{
			name: "considers child with mysql db in parent recursive",
			args: args{
				datasets: map[string][]zfs.Dataset{
					"included": {
						{
							Name: "tank",
						},
						{
							Name: "tank/a",
						},
						{
							Name: "tank/a/1",
						},
						{
							Name: "tank/a/2",
						},
						{
							Name: "tank/b",
						},
						{
							Name: "tank/b/1",
							DB:   "mysql",
						},
						{
							Name: "tank/b/2",
						},
					},
					"excluded": nil,
				},
			},
			want: map[string][]zfs.Dataset{
				"single": nil,
				"recursive": {
					{
						Name: "tank",
						DB:   "mysql",
					},
				},
				"included": {
					{
						Name: "tank",
					},
					{
						Name: "tank/a",
					},
					{
						Name: "tank/a/1",
					},
					{
						Name: "tank/a/2",
					},
					{
						Name: "tank/b",
					},
					{
						Name: "tank/b/1",
						DB:   "mysql",
					},
					{
						Name: "tank/b/2",
					},
				},
				"excluded": nil,
			},
		},
		{
			name: "considers child with mysql db in recursive with singles and exclusions",
			args: args{
				datasets: map[string][]zfs.Dataset{
					"included": {
						{
							Name: "tank",
						},
						{
							Name: "tank/a",
						},
						{
							Name: "tank/a/1",
						},
						{
							Name: "tank/a/2",
							DB:   "mysql",
						},
						{
							Name: "tank/b",
						},
						{
							Name: "tank/b/1",
						},
					},
					"excluded": {
						{
							Name: "tank/b/2",
						},
					},
				},
			},
			want: map[string][]zfs.Dataset{
				"single": {
					{
						Name: "tank",
					},
					{
						Name: "tank/b",
					},
				},
				"recursive": {
					{
						Name: "tank/a",
						DB:   "mysql",
					},
					{
						Name: "tank/b/1",
					},
				},
				"included": {
					{
						Name: "tank",
					},
					{
						Name: "tank/a",
					},
					{
						Name: "tank/a/1",
					},
					{
						Name: "tank/a/2",
						DB:   "mysql",
					},
					{
						Name: "tank/b",
					},
					{
						Name: "tank/b/1",
					},
				},
				"excluded": {
					{
						Name: "tank/b/2",
					},
				},
			},
		},
		{
			name: "considers child with mysql db in single with recursives and exclusions",
			args: args{
				datasets: map[string][]zfs.Dataset{
					"included": {
						{
							Name: "tank",
						},
						{
							Name: "tank/a",
						},
						{
							Name: "tank/a/1",
						},
						{
							Name: "tank/a/2",
						},
						{
							Name: "tank/b",
						},
						{
							Name: "tank/b/1",
							DB:   "mysql",
						},
					},
					"excluded": {
						{
							Name: "tank/b/2",
						},
					},
				},
			},
			want: map[string][]zfs.Dataset{
				"single": {
					{
						Name: "tank",
					},
					{
						Name: "tank/b",
					},
				},
				"recursive": {
					{
						Name: "tank/a",
					},
					{
						Name: "tank/b/1",
						DB:   "mysql",
					},
				},
				"included": {
					{
						Name: "tank",
					},
					{
						Name: "tank/a",
					},
					{
						Name: "tank/a/1",
					},
					{
						Name: "tank/a/2",
					},
					{
						Name: "tank/b",
					},
					{
						Name: "tank/b/1",
						DB:   "mysql",
					},
				},
				"excluded": {
					{
						Name: "tank/b/2",
					},
				},
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assertRecursiveDatasets(t, testCase.args.datasets, testCase.want)
		})
	}
}

func assertRecursiveDatasets(t *testing.T, datasets, want map[string][]zfs.Dataset) {
	t.Helper()

	got := findRecursiveDatasets(datasets)

	diff := deep.Equal(got, want)
	if diff != nil {
		t.Errorf("compare failed: %#v", diff)
	}
}

func Test_snapshotPrefix(t *testing.T) {
	type args struct {
		cfg config.Config
	}

	tests := []struct {
		name string
		want string
		args args
	}{
		{
			name: "prefixNotSet",
			args: args{
				config.Config{
					SnapshotPrefix: "",
				},
			},
			want: "zfs-auto-snap",
		},
		{
			name: "prefixSet",
			args: args{
				config.Config{
					SnapshotPrefix: "custom-snapshot-prefix",
				},
			},
			want: "custom-snapshot-prefix",
		},
	}

	t.Parallel()

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := snapshotPrefix(testCase.args.cfg)
			if got != testCase.want {
				t.Errorf("snapshotPrefix() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func Test_snapshotName(t *testing.T) {
	type args struct {
		cfg config.Config
	}

	tests := []struct {
		name string
		want string
		args args
	}{
		{
			name: "doNotUseUTC",
			args: args{
				cfg: config.Config{
					Timestamp: time.Date(2025, 05, 05, 17, 45, 0, 0,
						time.FixedZone("US/Eastern", 0)),
					Interval:       "frequent",
					SnapshotPrefix: "",
					UseUTC:         false,
				},
			},
			want: "zfs-auto-snap_frequent-2025-05-05-17h45",
		},
		{
			name: "useUTC",
			args: args{
				cfg: config.Config{
					Timestamp: time.Date(2025, 05, 05, 17, 45, 0, 0,
						time.FixedZone("US/Eastern", 0)),
					Interval:       "frequent",
					SnapshotPrefix: "",
					UseUTC:         true,
				},
			},
			want: "zfs-auto-snap_frequent-2025-05-05-17h45U",
		},
		{
			name: "doNotUseUTCTestTimeIsUTC",
			args: args{
				cfg: config.Config{
					Timestamp: time.Date(2025, 05, 05, 17, 45, 0, 0,
						time.UTC),
					Interval:       "frequent",
					SnapshotPrefix: "",
					UseUTC:         false,
				},
			},
			want: "zfs-auto-snap_frequent-2025-05-05-17h45",
		},
		{
			name: "useUTCTestTimeIsUTC",
			args: args{
				cfg: config.Config{
					Timestamp: time.Date(2025, 05, 05, 17, 45, 0, 0,
						time.UTC),
					Interval:       "frequent",
					SnapshotPrefix: "",
					UseUTC:         true,
				},
			},
			want: "zfs-auto-snap_frequent-2025-05-05-17h45U",
		},
	}

	t.Parallel()

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := snapshotName(testCase.args.cfg)
			if got != testCase.want {
				t.Errorf("snapshotName() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestDestroyZeroSizedSnapshots(t *testing.T) {
	t.Parallel()

	type args struct {
		snaps []zfs.Snapshot
		cfg   config.Config
	}

	tests := []struct {
		name string
		want []zfs.Snapshot
		args args
	}{
		{
			name: "zeroSnapshots",
			args: args{
				snaps: nil,
				cfg:   config.Config{},
			},
		},
		{
			name: "oneSnapshotNotZero",
			args: args{
				snaps: []zfs.Snapshot{
					{
						Name: "tank/a@1",
						Used: 123456,
					},
				},
				cfg: config.Config{},
			},
			want: []zfs.Snapshot{
				{
					Name: "tank/a@1",
					Used: 123456,
				},
			},
		},
		{
			name: "oneSnapshotZero",
			args: args{
				snaps: []zfs.Snapshot{
					{
						Name: "tank/a@1",
						Used: 0,
					},
				},
				cfg: config.Config{},
			},
			want: []zfs.Snapshot{
				{
					Name: "tank/a@1",
					Used: 0,
				},
			},
		},
		{
			name: "twoSnapshotsNeitherZero",
			args: args{
				snaps: []zfs.Snapshot{
					{
						Name: "tank/a@2",
						Used: 123456,
					},
					{
						Name: "tank/a@1",
						Used: 12345,
					},
				},
				cfg: config.Config{},
			},
			want: []zfs.Snapshot{
				{
					Name: "tank/a@2",
					Used: 123456,
				},
				{
					Name: "tank/a@1",
					Used: 12345,
				},
			},
		},
		{
			name: "twoSnapshotsFirstZero",
			args: args{
				snaps: []zfs.Snapshot{
					{
						Name: "tank/a@2",
						Used: 123456,
					},
					{
						Name: "tank/a@1",
						Used: 0,
					},
				},
				cfg: config.Config{},
			},
			want: []zfs.Snapshot{
				{
					Name: "tank/a@2",
					Used: 123456,
				},
			},
		},
		{
			name: "twoSnapshotsSecondZero",
			args: args{
				snaps: []zfs.Snapshot{
					{
						Name: "tank/a@2",
						Used: 0,
					},
					{
						Name: "tank/a@1",
						Used: 123456,
					},
				},
				cfg: config.Config{},
			},
			want: []zfs.Snapshot{
				{
					Name: "tank/a@2",
					Used: 0,
				},
				{
					Name: "tank/a@1",
					Used: 123456,
				},
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var listing bytes.Buffer
			for _, snapshot := range testCase.args.snaps {
				_, _ = fmt.Fprintf(&listing, "%s\t%d\n", snapshot.Name, snapshot.Used)
			}

			runner := &fakeRunner{output: listing.Bytes()}
			client := zfs.NewClient(runner, io.Discard)

			snapshots, err := client.ListSnapshots("", true, false)
			if err != nil {
				t.Fatalf("ListSnapshots() error = %v", err)
			}

			got := New(client, io.Discard).destroyZeroSizedSnapshots(snapshots, testCase.args.cfg)
			for i := range got {
				got[i] = zfs.Snapshot{Name: got[i].Name, Used: got[i].Used}
			}

			diff := deep.Equal(got, testCase.want)
			if diff != nil {
				t.Errorf("compare failed: %#v", diff)
			}
		})
	}
}

func TestDestroyZeroSizedSnapshotsVerboseOutput(t *testing.T) {
	t.Parallel()

	output := &bytes.Buffer{}
	runner := &fakeRunner{output: []byte("tank/a@2\t0\ntank/a@1\t0\n")}
	client := zfs.NewClient(runner, output)

	snapshots, err := client.ListSnapshots("", true, false)
	if err != nil {
		t.Fatalf("ListSnapshots() error = %v", err)
	}

	New(client, output).destroyZeroSizedSnapshots(snapshots, config.Config{DryRun: true, Verbose: true})

	want := "Destroying zero-sized snapshot: tank/a@1\n"
	if got := output.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestDatasetsDestroyZeroSizedSnapshotsParallel(t *testing.T) {
	t.Parallel()

	var getUsed sync.WaitGroup
	getUsed.Add(2)

	runner := &fakeRunner{
		runFunc: func(name string, args ...string) ([]byte, error) {
			if name == "zfs" && len(args) > 0 {
				switch args[0] {
				case "list":
					return []byte("tank/a@2\t1\ntank/a@1\t0\ntank/b@2\t1\ntank/b@1\t0\n"), nil
				case "get":
					getUsed.Done()
					getUsed.Wait()

					return []byte("0\n"), nil
				}
			}

			return nil, nil
		},
	}
	client := zfs.NewClient(runner, io.Discard)
	tools := New(client, io.Discard)

	snapshots, err := client.ListSnapshots("", true, false)
	if err != nil {
		t.Fatalf("ListSnapshots() error = %v", err)
	}

	grouped := GroupSnapshotsIntoDatasets(snapshots, []zfs.Dataset{{Name: "tank/a"}, {Name: "tank/b"}})

	got := tools.DatasetsDestroyZeroSizedSnapshots(grouped, config.Config{UseThreads: true})
	for name, snapshots := range got {
		if len(snapshots) != 1 || snapshots[0].Name != name+"@2" {
			t.Errorf("snapshots for %s = %#v, want newest snapshot only", name, snapshots)
		}
	}

	var destroyed []string

	for _, call := range runner.calls {
		if call.name == "zfs" && len(call.args) > 0 && call.args[0] == "destroy" {
			destroyed = append(destroyed, call.args[len(call.args)-1])
		}
	}

	slices.Sort(destroyed)

	if diff := deep.Equal(destroyed, []string{"tank/a@1", "tank/b@1"}); diff != nil {
		t.Errorf("destroyed snapshots differ: %v", diff)
	}
}

func findEligibleDatasetsOutput(name string) []byte {
	var (
		output       string
		enabledTwice = strings.Repeat("\ttrue", 2)
	)

	switch name {
	case "TestFindEligibleDatasets_noExistingSnapshotsOneDataset":
		output = "tank/fs1\tfilesystem\t-\ttrue\tyes\n"
	case "TestFindEligibleDatasets_noExistingSnapshotsTwoDatasetsOneUnmounted":
		output = "tank/fs1\tfilesystem\t-\ttrue\tyes\n" +
			"tank/fs2\tfilesystem\t-\ttrue\tno\n"
	case "TestFindEligibleDatasets_alreadyFound":
		output = "tank/fs1\tfilesystem\t-\ttrue\tyes\n" +
			"tank/fs2\tfilesystem" + enabledTwice + "\tyes\n"
	case "TestFindEligibleDatasets_onlyFreq":
		output = "tank/fs1\tfilesystem\t-\ttrue\tyes\n" +
			"tank/fs2\tfilesystem" + enabledTwice + "\tyes\n" +
			"tank/fs3\tfilesystem\ttrue\t-\tyes\n" +
			"tank/fs4\tfilesystem\t-\t-\tyes\n"
	case "TestFindEligibleDatasets_manyDatasets":
		output = `tank	filesystem	-	-	yes
tank/ROOT	filesystem	-	-	no
tank/ROOT/default	filesystem	-	true	yes
tank/poudriere	filesystem	-	false	yes
tank/poudriere/ccache	filesystem	-	false	yes
tank/poudriere/data	filesystem	-	false	yes
tank/poudriere/data/cache	filesystem	-	false	yes
tank/poudriere/data/logs	filesystem	-	false	yes
tank/poudriere/data/packages	filesystem	-	false	yes
tank/poudriere/data/wrkdirs	filesystem	-	false	yes
tank/poudriere/distfiles	filesystem	-	false	yes
tank/poudriere/jails	filesystem	-	false	yes
tank/poudriere/jails/head-amd64	filesystem	-	false	yes
tank/poudriere/ports	filesystem	-	false	yes
tank/poudriere/ports/default	filesystem	-	true	yes
tank/tmp	filesystem	-	-	yes
tank/usr	filesystem	-	-	no
tank/usr/home	filesystem	-	true	yes
tank/usr/obj	filesystem	-	-	yes
tank/usr/src	filesystem	-	-	yes
tank/var	filesystem	-	-	no
tank/var/audit	filesystem	-	-	yes
tank/var/crash	filesystem	-	-	yes
tank/var/log	filesystem	-	-	yes
tank/var/mail	filesystem	-	-	yes
tank/var/tmp	filesystem	-	-	yes
tank/moredata	filesystem	-	true	yes
tank/moredata/2	filesystem	-	false	yes
tank/moredata/3	filesystem	-	true	yes
`
	}

	return []byte(output)
}
