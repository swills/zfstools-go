package zfstools

import (
	"fmt"
	"os"
	"testing"

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

func TestGroupSnapshotsIntoDatasets(t *testing.T) {
	t.Parallel()

	datasets := []zfs.Dataset{
		{Name: "pool/home"},
		{Name: "pool/data"},
	}
	snaps := []zfs.Snapshot{
		{Name: "pool/home@zfs-auto-snap_hourly-2025-01-01-01h00"},
		{Name: "pool/data@zfs-auto-snap_hourly-2025-01-01-01h00"},
	}
	grouped := GroupSnapshotsIntoDatasets(snaps, datasets)

	if len(grouped["pool/home"]) != 1 || len(grouped["pool/data"]) != 1 {
		t.Error("expected each dataset to have one snapshot")
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
