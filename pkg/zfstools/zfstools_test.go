package zfstools

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-test/deep"

	"zfstools-go/pkg/config"
	"zfstools-go/pkg/zfs"
	"zfstools-go/pkg/zfstoolstest"
)

var createdSnapshots []string

var destroyedSnapshots []string

func init() {
	createdSnapshots = nil
	destroyedSnapshots = nil
	createManySnapshotsFn = func(name string, datasets []zfs.Dataset, _, _, _, _, _ bool) error {
		for _, ds := range datasets {
			createdSnapshots = append(createdSnapshots, ds.Name+"@"+name)
		}

		return nil
	}
	destroySnapshotFn = func(name string, _, _ bool) error {
		destroyedSnapshots = append(destroyedSnapshots, name)

		return nil
	}
}

func testConfig(interval string) config.Config {
	return config.Config{
		Interval: interval,
	}
}

func TestDoNewSnapshots(t *testing.T) {
	t.Parallel()

	createdSnapshots = nil
	cfg := testConfig("frequent")
	datasets := map[string][]zfs.Dataset{
		"single":    {{Name: "pool/fs1"}},
		"recursive": {{Name: "pool/fs2"}},
	}
	DoNewSnapshots(cfg, datasets)

	if len(createdSnapshots) != 2 {
		t.Errorf("expected 2 snapshots, got %d", len(createdSnapshots))
	}
}

//nolint:paralleltest
func TestGroupSnapshotsIntoDatasets(t *testing.T) {
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
			got := GroupSnapshotsIntoDatasets(testCase.args.snaps, testCase.args.datasets)

			diff := deep.Equal(got, testCase.want)
			if diff != nil {
				t.Errorf("compare failed: %#v", diff)
			}
		})
	}
}

//nolint:paralleltest,maintidx
func TestFindEligibleDatasets(t *testing.T) {
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
			zfs.RunZfsFn = zfstoolstest.MakeFakeCommand(testCase.mockCmdFunc)

			got := FindEligibleDatasets(testCase.args.cfg, testCase.args.pool)

			diff := deep.Equal(got, testCase.want)
			if diff != nil {
				t.Errorf("compare failed: %#v", diff)
			}
		})
	}
}

//nolint:paralleltest,maintidx
func Test_findRecursiveDatasets(t *testing.T) {
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
			got := findRecursiveDatasets(testCase.args.datasets)

			diff := deep.Equal(got, testCase.want)
			if diff != nil {
				t.Errorf("compare failed: %#v", diff)
			}
		})
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

// test helpers from here down

//nolint:paralleltest
func TestFindEligibleDatasets_noExistingSnapshotsOneDataset(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"zfs",
		"list",
		"-H",
		"-t",
		"filesystem,volume",
		"-o",
		"name,type,com.sun:auto-snapshot:frequent,com.sun:auto-snapshot,mounted",
		"-s",
		"name",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	fmt.Printf("tank/fs1\tfilesystem\t-\ttrue\tyes\n") //nolint:forbidigo

	os.Exit(0)
}

//nolint:paralleltest
func TestFindEligibleDatasets_noExistingSnapshotsTwoDatasetsOneUnmounted(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"zfs",
		"list",
		"-H",
		"-t",
		"filesystem,volume",
		"-o",
		"name,type,com.sun:auto-snapshot:frequent,com.sun:auto-snapshot,mounted",
		"-s",
		"name",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	fmt.Printf("tank/fs1\tfilesystem\t-\ttrue\tyes\n") //nolint:forbidigo
	fmt.Printf("tank/fs2\tfilesystem\t-\ttrue\tno\n")  //nolint:forbidigo

	os.Exit(0)
}

//nolint:paralleltest
func TestFindEligibleDatasets_alreadyFound(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"zfs",
		"list",
		"-H",
		"-t",
		"filesystem,volume",
		"-o",
		"name,type,com.sun:auto-snapshot:frequent,com.sun:auto-snapshot,mounted",
		"-s",
		"name",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	fmt.Printf("tank/fs1\tfilesystem\t-\ttrue\tyes\n")    //nolint:forbidigo
	fmt.Printf("tank/fs2\tfilesystem\ttrue\ttrue\tyes\n") //nolint:forbidigo,dupword

	os.Exit(0)
}

//nolint:paralleltest
func TestFindEligibleDatasets_onlyFreq(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"zfs",
		"list",
		"-H",
		"-t",
		"filesystem,volume",
		"-o",
		"name,type,com.sun:auto-snapshot:frequent,com.sun:auto-snapshot,mounted",
		"-s",
		"name",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	fmt.Printf("tank/fs1\tfilesystem\t-\ttrue\tyes\n")    //nolint:forbidigo
	fmt.Printf("tank/fs2\tfilesystem\ttrue\ttrue\tyes\n") //nolint:forbidigo,dupword
	fmt.Printf("tank/fs3\tfilesystem\ttrue\t-\tyes\n")    //nolint:forbidigo
	fmt.Printf("tank/fs4\tfilesystem\t-\t-\tyes\n")       //nolint:forbidigo

	os.Exit(0)
}

//nolint:paralleltest
func TestFindEligibleDatasets_manyDatasets(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"zfs",
		"list",
		"-H",
		"-t",
		"filesystem,volume",
		"-o",
		"name,type,com.sun:auto-snapshot:frequent,com.sun:auto-snapshot,mounted",
		"-s",
		"name",
		"-r",
		"tank",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	//nolint:forbidigo
	fmt.Printf(`tank	filesystem	-	-	yes
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
`)

	os.Exit(0)
}
