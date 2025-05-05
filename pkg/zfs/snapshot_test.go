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
func TestSnapshot_IsZero(t *testing.T) {
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
		args        args
		want        bool
	}{
		{
			name: "true",
			fields: fields{
				Name: "pool1/fs1@snap",
				Used: 0,
			},
			mockCmdFunc: "TestSnapshot_IsZeroTrue",
			args:        args{debug: false},
			want:        true,
		},
		{
			name: "false",
			fields: fields{
				Name: "pool1/fs2@snap",
				Used: 123,
			},
			mockCmdFunc: "TestSnapshot_IsZeroTrue", // not used
			args:        args{debug: false},
			want:        false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			staleSnapshotSize = false
			runZfsFn = zfstoolstest.MakeFakeCommand(testCase.mockCmdFunc)

			s := &Snapshot{
				Name: testCase.fields.Name,
				Used: testCase.fields.Used,
			}

			got := s.IsZero(testCase.args.debug)
			if got != testCase.want {
				t.Errorf("IsZero() = %v, want %v", got, testCase.want)
			}
		})
	}
}

//nolint:paralleltest
func TestListSnapshots(t *testing.T) {
	type args struct {
		dataset   string
		recursive bool
		debug     bool
	}

	tests := []struct {
		name        string
		args        args
		mockCmdFunc string
		want        []Snapshot
		wantErr     bool
	}{
		{
			name:        "getAllNoneFound",
			mockCmdFunc: "TestListSnapshots_getAllNoneFound",
			args: args{
				dataset:   "",
				recursive: false,
				debug:     false,
			},
			want:    []Snapshot{},
			wantErr: false,
		},
		{
			name:        "getAllOneFound",
			mockCmdFunc: "TestListSnapshots_getAllOneFound",
			args: args{
				dataset:   "",
				recursive: false,
				debug:     false,
			},
			want: []Snapshot{
				{
					Name: "tank/data@backup",
					Used: 134217728,
				},
			},
			wantErr: false,
		},
		{
			name:        "getOneNoneFound",
			mockCmdFunc: "TestListSnapshots_getOneNoneFound",
			args: args{
				dataset:   "tank",
				recursive: false,
				debug:     false,
			},
			want:    []Snapshot{},
			wantErr: false,
		},
		{
			name:        "getOneOneFound",
			mockCmdFunc: "TestListSnapshots_getOneOneFound",
			args: args{
				dataset:   "tank",
				recursive: false,
				debug:     false,
			},
			want: []Snapshot{
				{
					Name: "tank/data@backup1",
					Used: 131072,
				},
			},
			wantErr: false,
		},
		{
			name:        "getAllRecursiveNoneFound",
			mockCmdFunc: "TestListSnapshots_getAllRecursiveNoneFound",
			args: args{
				dataset:   "",
				recursive: true,
				debug:     false,
			},
			want:    []Snapshot{},
			wantErr: false,
		},
		{
			name:        "getAllRecursiveOneFound",
			mockCmdFunc: "TestListSnapshots_getAllRecursiveOneFound",
			args: args{
				dataset:   "",
				recursive: true,
				debug:     false,
			},
			want: []Snapshot{
				{
					Name: "tank/data@backup",
					Used: 134217728,
				},
			},
			wantErr: false,
		},
		{
			name:        "getOneRecursiveNoneFound",
			mockCmdFunc: "TestListSnapshots_getOneRecursiveNoneFound",
			args: args{
				dataset:   "tank",
				recursive: true,
				debug:     false,
			},
			want:    []Snapshot{},
			wantErr: false,
		},
		{
			name:        "getOneRecursiveOneFound",
			mockCmdFunc: "TestListSnapshots_getOneRecursiveOneFound",
			args: args{
				dataset:   "tank",
				recursive: true,
				debug:     false,
			},
			want: []Snapshot{
				{
					Name: "tank/data@backup1",
					Used: 131072,
				},
			},
			wantErr: false,
		},
		{
			name:        "getAllOneFoundBogusSize",
			mockCmdFunc: "TestListSnapshots_getAllOneFoundBogusSize",
			args: args{
				dataset:   "",
				recursive: false,
				debug:     false,
			},
			want:    []Snapshot{},
			wantErr: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			runZfsFn = zfstoolstest.MakeFakeCommand(testCase.mockCmdFunc)

			got, err := ListSnapshots(testCase.args.dataset, testCase.args.recursive, testCase.args.debug)

			if (err != nil) != testCase.wantErr {
				t.Errorf("ListSnapshots() error = %v, wantErr %v", err, testCase.wantErr)

				return
			}

			diff := deep.Equal(got, testCase.want)
			if diff != nil {
				t.Errorf("compare failed: %#v", diff)
			}
		})
	}
}

//nolint:paralleltest
func TestCreateSnapshot(t *testing.T) {
	type args struct {
		dbName    string
		targets   []string
		recursive bool
		dryRun    bool
		verbose   bool
		debug     bool
	}

	tests := []struct {
		name        string
		mockCmdFunc string
		args        args
		wantErr     bool
	}{
		{
			name:        "none",
			mockCmdFunc: "TestCreateSnapshot_none", // shouldn't be called
			args: args{
				targets:   nil,
				recursive: false,
				dbName:    "",
				dryRun:    false,
				verbose:   false,
				debug:     false,
			},
			wantErr: true,
		},
		{
			name:        "empty",
			mockCmdFunc: "TestCreateSnapshot_none", // shouldn't be called
			args: args{
				targets:   []string{""},
				recursive: false,
				dbName:    "",
				dryRun:    false,
				verbose:   false,
				debug:     false,
			},
			wantErr: true,
		},
		{
			name:        "noAt",
			mockCmdFunc: "TestCreateSnapshot_none", // shouldn't be called
			args: args{
				targets:   []string{"noAtSignWhichShouldBePresent"},
				recursive: false,
				dbName:    "",
				dryRun:    false,
				verbose:   false,
				debug:     false,
			},
			wantErr: true,
		},
		{
			name:        "simple",
			mockCmdFunc: "TestCreateSnapshot_single",
			args: args{
				targets:   []string{"pool/fs@snap"},
				recursive: false,
				dbName:    "",
				dryRun:    false,
				verbose:   false,
				debug:     false,
			},
			wantErr: false,
		},
		{
			name:        "multiple",
			mockCmdFunc: "TestCreateSnapshot_multiple",
			args: args{
				targets:   []string{"pool/fs1@snap", "pool1/fs2@snap"},
				recursive: false,
				dbName:    "",
				dryRun:    false,
				verbose:   false,
				debug:     false,
			},
			wantErr: false,
		},
		{
			name:        "simpleRecursive",
			mockCmdFunc: "TestCreateSnapshot_singleRecursive",
			args: args{
				targets:   []string{"pool/fs@snap"},
				recursive: true,
				dbName:    "",
				dryRun:    false,
				verbose:   false,
				debug:     false,
			},
			wantErr: false,
		},
		{
			name:        "multipleRecursive",
			mockCmdFunc: "TestCreateSnapshot_multipleRecursive",
			args: args{
				targets:   []string{"pool/fs1@snap", "pool1/fs2@snap"},
				recursive: true,
				dbName:    "",
				dryRun:    false,
				verbose:   false,
				debug:     false,
			},
			wantErr: false,
		},
		{
			name:        "mySQLsingle",
			mockCmdFunc: "TestCreateSnapshot_mySQLsingle",
			args: args{
				targets:   []string{"pool/fs@snap"},
				recursive: false,
				dbName:    "mysql",
				dryRun:    false,
				verbose:   false,
				debug:     false,
			},
			wantErr: false,
		},
		{
			name:        "postgreSQLsingle",
			mockCmdFunc: "TestCreateSnapshot_postgreSQLsingle",
			args: args{
				targets:   []string{"pool/fs@snap"},
				recursive: false,
				dbName:    "postgresql",
				dryRun:    false,
				verbose:   false,
				debug:     false,
			},
			wantErr: false,
		},
		{
			name:        "dryRun",
			mockCmdFunc: "TestCreateSnapshot_none", // shouldn't be called
			args: args{
				targets:   []string{"pool/fs@snap"},
				recursive: false,
				dbName:    "",
				dryRun:    true,
				verbose:   false,
				debug:     false,
			},
			wantErr: false,
		},
		{
			name:        "forceError",
			mockCmdFunc: "TestCreateSnapshot_forceError",
			args: args{
				targets:   []string{"pool/fs@snap"},
				recursive: false,
				dbName:    "",
				dryRun:    false,
				verbose:   false,
				debug:     false,
			},
			wantErr: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			runZfsFn = zfstoolstest.MakeFakeCommand(testCase.mockCmdFunc)

			err := CreateSnapshot(testCase.args.targets, testCase.args.recursive, testCase.args.dbName,
				testCase.args.dryRun, testCase.args.verbose, testCase.args.debug)

			if (err != nil) != testCase.wantErr {
				t.Errorf("CreateSnapshot() error = %v, wantErr %v", err, testCase.wantErr)

				return
			}
		})
	}
}

//nolint:paralleltest
func TestCreateManySnapshots(t *testing.T) {
	type args struct {
		snapshotName string
		datasets     []Dataset
		recursive    bool
		dryRun       bool
		verbose      bool
		debug        bool
		useThreads   bool
	}

	tests := []struct {
		name        string
		mockCmdFunc string
		args        args
		bookmarks   bool
		wantErr     bool
	}{
		{
			name:        "emptySnapName",
			mockCmdFunc: "TestCreateManySnapshots_simple", // shouldn't be called
			args: args{
				snapshotName: "",
				datasets: []Dataset{
					{Name: "pool/fs1"},
					{Name: "pool/fs2"},
				},
				recursive:  false,
				dryRun:     false,
				verbose:    false,
				debug:      false,
				useThreads: false,
			},
			wantErr: true,
		},
		{
			name:        "nilDatasets",
			mockCmdFunc: "TestCreateManySnapshots_simple", // shouldn't be called
			args: args{
				snapshotName: "auto-2025-01-01",
				datasets:     nil,
				recursive:    false,
				dryRun:       false,
				verbose:      false,
				debug:        false,
				useThreads:   false,
			},
			wantErr: true,
		},
		{
			name:        "datasetNameEmpty",
			mockCmdFunc: "TestCreateManySnapshots_simple", // shouldn't be called
			args: args{
				snapshotName: "auto-2025-01-01",
				datasets: []Dataset{
					{Name: "pool/fs1"},
					{Name: ""},
				},
				recursive:  false,
				dryRun:     false,
				verbose:    false,
				debug:      false,
				useThreads: false,
			},
			wantErr: true,
		},
		{
			name:        "datasetNameContainsAt",
			mockCmdFunc: "TestCreateManySnapshots_simple", // shouldn't be called
			args: args{
				snapshotName: "auto-2025-01-01",
				datasets: []Dataset{
					{Name: "pool/fs1"},
					{Name: "pool/fs2@snapname"},
				},
				recursive:  false,
				dryRun:     false,
				verbose:    false,
				debug:      false,
				useThreads: false,
			},
			wantErr: true,
		},
		{
			name:        "simpleWithBookmarks",
			mockCmdFunc: "TestCreateManySnapshots_simpleWithBookmarks",
			bookmarks:   true,
			args: args{
				snapshotName: "auto-2025-01-01",
				datasets: []Dataset{
					{Name: "pool/fs1"},
					{Name: "pool/fs2"},
				},
				recursive:  false,
				dryRun:     false,
				verbose:    false,
				debug:      false,
				useThreads: false,
			},
			wantErr: false,
		},
		{
			name:        "simpleWithoutBookmarks",
			mockCmdFunc: "TestCreateManySnapshots_simpleWithoutBookmarks",
			bookmarks:   false,
			args: args{
				snapshotName: "auto-2025-01-01",
				datasets: []Dataset{
					{Name: "pool/fs1"},
					{Name: "pool/fs2"},
				},
				recursive:  false,
				dryRun:     false,
				verbose:    false,
				debug:      false,
				useThreads: false,
			},
			wantErr: false,
		},
		{
			name:        "oneSnapshotOfManyErroredWithBookmarks",
			mockCmdFunc: "TestCreateManySnapshots_oneSnapshotOfManyErroredWithBookmarks",
			bookmarks:   true,
			args: args{
				snapshotName: "auto-2025-01-01",
				datasets: []Dataset{
					{Name: "pool/fs1"},
					{Name: "pool/fs2"},
				},
				recursive:  false,
				dryRun:     false,
				verbose:    false,
				debug:      false,
				useThreads: false,
			},
			wantErr: true,
		},
		{
			name:        "oneSnapshotOfManyErroredWithoutBookmarks",
			mockCmdFunc: "TestCreateManySnapshots_oneSnapshotOfManyErroredWithoutBookmarks",
			bookmarks:   false,
			args: args{
				snapshotName: "auto-2025-01-01",
				datasets: []Dataset{
					{Name: "pool/fs1"},
					{Name: "pool/fs2"},
				},
				recursive:  false,
				dryRun:     false,
				verbose:    false,
				debug:      false,
				useThreads: false,
			},
			wantErr: true,
		},
	}

	for _, testCase := range tests {
		runZfsFn = zfstoolstest.MakeFakeCommand(testCase.mockCmdFunc)
		// ensure we control for bookmark feature support detection
		runZpoolFn = zfstoolstest.MakeFakeCommand(testCase.mockCmdFunc)

		t.Run(testCase.name, func(t *testing.T) {
			// bookmark/multisnap support may have been detected already (on or off), but make sure we force it to
			// what we need for this test case
			if testCase.bookmarks {
				haveBookmarks = true
				haveMultiSnap = true
			} else {
				haveBookmarks = false
				haveMultiSnap = false
			}

			err := CreateManySnapshots(testCase.args.snapshotName, testCase.args.datasets,
				testCase.args.recursive, testCase.args.dryRun, testCase.args.verbose,
				testCase.args.debug, testCase.args.useThreads)

			if (err != nil) != testCase.wantErr {
				t.Errorf("CreateManySnapshots() error = %v, wantErr %v", err, testCase.wantErr)

				return
			}
		})
	}
}

//nolint:paralleltest
func Test_getArgMax(t *testing.T) {
	tests := []struct {
		name        string
		mockCmdFunc string
		want        int
	}{
		{
			name:        "working",
			mockCmdFunc: "Test_getArgMax_working",
			want:        123456,
		},
		{
			name:        "error",
			mockCmdFunc: "Test_getArgMax_error",
			want:        4096,
		},
		{
			name:        "working",
			mockCmdFunc: "Test_getArgMax_bogus",
			want:        4096,
		},
	}

	for _, testCase := range tests {
		runZfsFn = zfstoolstest.MakeFakeCommand(testCase.mockCmdFunc)

		t.Run(testCase.name, func(t *testing.T) {
			got := getArgMax()
			if got != testCase.want {
				t.Errorf("getArgMax() = %v, want %v", got, testCase.want)
			}
		})
	}
}

//nolint:paralleltest
func TestDestroySnapshot(t *testing.T) {
	type args struct {
		name   string
		dryRun bool
		debug  bool
	}

	tests := []struct {
		name        string
		mockCmdFunc string
		args        args
		wantErr     bool
	}{
		{
			name:        "working",
			mockCmdFunc: "TestDestroySnapshot_working",
			args: args{
				name:   "pool1/fs1@snapshot1",
				dryRun: false,
				debug:  false,
			},
			wantErr: false,
		},
		{
			name:        "error",
			mockCmdFunc: "TestDestroySnapshot_error",
			args: args{
				name:   "pool1/fs1@snapshot1",
				dryRun: false,
				debug:  false,
			},
			wantErr: true,
		},
		{
			name:        "dryRun",
			mockCmdFunc: "TestDestroySnapshot_dryRun", // not called
			args: args{
				name:   "pool1/fs1@snapshot1",
				dryRun: true,
				debug:  false,
			},
			wantErr: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			staleSnapshotSize = false
			runZfsFn = zfstoolstest.MakeFakeCommand(testCase.mockCmdFunc)

			err := DestroySnapshot(testCase.args.name, testCase.args.dryRun, testCase.args.debug)
			if (err != nil) != testCase.wantErr {
				t.Errorf("DestroySnapshot() error = %v, wantErr %v", err, testCase.wantErr)
			}

			if staleSnapshotSize != true {
				t.Errorf("staleSnapshotSize not updated")
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

//nolint:paralleltest
func TestSnapshot_IsZeroTrue(_ *testing.T) {
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
		"pool1/fs1@snap",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	fmt.Printf("0\n") //nolint:forbidigo

	os.Exit(0)
}

//nolint:paralleltest
func TestListSnapshots_getAllNoneFound(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"zfs",
		"list",
		"-H",
		"-p",
		"-t",
		"snapshot",
		"-o",
		"name,used",
		"-S",
		"name",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	fmt.Printf("\n") //nolint:forbidigo

	os.Exit(0)
}

//nolint:paralleltest
func TestListSnapshots_getAllOneFound(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"zfs",
		"list",
		"-H",
		"-p",
		"-t",
		"snapshot",
		"-o",
		"name,used",
		"-S",
		"name",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	fmt.Printf("tank/data@backup\t134217728\n") //nolint:forbidigo

	os.Exit(0)
}

//nolint:paralleltest
func TestListSnapshots_getOneNoneFound(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"zfs",
		"list",
		"-d",
		"1",
		"-H",
		"-p",
		"-t",
		"snapshot",
		"-o",
		"name,used",
		"-S",
		"name",
		"tank",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	fmt.Printf("\n") //nolint:forbidigo

	os.Exit(0)
}

//nolint:paralleltest
func TestListSnapshots_getOneOneFound(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"zfs",
		"list",
		"-d",
		"1",
		"-H",
		"-p",
		"-t",
		"snapshot",
		"-o",
		"name,used",
		"-S",
		"name",
		"tank",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	fmt.Printf("tank/data@backup1\t131072\n") //nolint:forbidigo

	os.Exit(0)
}

//nolint:paralleltest
func TestListSnapshots_getAllRecursiveNoneFound(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"zfs",
		"list",
		"-r",
		"-H",
		"-p",
		"-t",
		"snapshot",
		"-o",
		"name,used",
		"-S",
		"name",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	fmt.Printf("\n") //nolint:forbidigo

	os.Exit(0)
}

//nolint:paralleltest
func TestListSnapshots_getAllRecursiveOneFound(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"zfs",
		"list",
		"-r",
		"-H",
		"-p",
		"-t",
		"snapshot",
		"-o",
		"name,used",
		"-S",
		"name",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	fmt.Printf("tank/data@backup\t134217728\n") //nolint:forbidigo

	os.Exit(0)
}

//nolint:paralleltest
func TestListSnapshots_getOneRecursiveNoneFound(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"zfs",
		"list",
		"-r",
		"-H",
		"-p",
		"-t",
		"snapshot",
		"-o",
		"name,used",
		"-S",
		"name",
		"tank",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	fmt.Printf("\n") //nolint:forbidigo

	os.Exit(0)
}

//nolint:paralleltest
func TestListSnapshots_getOneRecursiveOneFound(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"zfs",
		"list",
		"-r",
		"-H",
		"-p",
		"-t",
		"snapshot",
		"-o",
		"name,used",
		"-S",
		"name",
		"tank",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	fmt.Printf("tank/data@backup1\t131072\n") //nolint:forbidigo

	os.Exit(0)
}

//nolint:paralleltest
func TestListSnapshots_getAllOneFoundBogusSize(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"zfs",
		"list",
		"-H",
		"-p",
		"-t",
		"snapshot",
		"-o",
		"name,used",
		"-S",
		"name",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	fmt.Printf("tank/data@backup\tonetwothree\n") //nolint:forbidigo

	os.Exit(0)
}

//nolint:paralleltest
func TestCreateSnapshot_none(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	os.Exit(1)
}

//nolint:paralleltest
func TestCreateSnapshot_single(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"sh",
		"-c",
		"zfs snapshot pool/fs@snap",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	os.Exit(0)
}

//nolint:paralleltest
func TestCreateSnapshot_multiple(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"sh",
		"-c",
		"zfs snapshot pool/fs1@snap pool1/fs2@snap",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	os.Exit(0)
}

//nolint:paralleltest
func TestCreateSnapshot_singleRecursive(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"sh",
		"-c",
		"zfs snapshot -r pool/fs@snap",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	os.Exit(0)
}

//nolint:paralleltest
func TestCreateSnapshot_multipleRecursive(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"sh",
		"-c",
		"zfs snapshot -r pool/fs1@snap pool1/fs2@snap",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	os.Exit(0)
}

//nolint:paralleltest
func TestCreateSnapshot_mySQLsingle(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"sh",
		"-c",
		"mysql -e \" FLUSH LOGS; FLUSH TABLES WITH READ LOCK; SYSTEM zfs snapshot pool/fs@snap; UNLOCK TABLES;\"",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	os.Exit(0)
}

//nolint:paralleltest
func TestCreateSnapshot_postgreSQLsingle(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"sh",
		"-c",
		"(psql -c \"SELECT PG_START_BACKUP('zfs-auto-snapshot');\" postgres ; zfs snapshot pool/fs@snap ) ; psql -c \"SELECT PG_STOP_BACKUP();\" postgres", //nolint:lll
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	os.Exit(0)
}

//nolint:paralleltest
func TestCreateSnapshot_forceError(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	os.Exit(1)
}

//nolint:paralleltest
func TestCreateManySnapshots_simpleWithBookmarks(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	// simulate the zpool call used to detect bookmarks feature
	for _, v := range cmdWithArgs {
		if v == "feature@bookmarks" {
			fmt.Printf("tank\tfeature@bookmarks\tenabled\n") //nolint:forbidigo
			os.Exit(0)
		}
	}

	expectedCmdWithArgs := []string{
		"sh",
		"-c",
		"zfs snapshot pool/fs1@auto-2025-01-01 pool/fs2@auto-2025-01-01",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	os.Exit(0)
}

//nolint:paralleltest
func TestCreateManySnapshots_simpleWithoutBookmarks(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	// simulate the zpool call used to detect bookmarks feature - this time without bookmarks supported
	for _, v := range cmdWithArgs {
		if v == "feature@bookmarks" {
			os.Exit(0)
		}
	}

	expectedFirstCmdWithArgs := []string{
		"sh",
		"-c",
		"zfs snapshot pool/fs1@auto-2025-01-01",
	}

	expectedSecondCmdWithArgs := []string{
		"sh",
		"-c",
		"zfs snapshot pool/fs2@auto-2025-01-01",
	}

	if deep.Equal(cmdWithArgs, expectedFirstCmdWithArgs) == nil ||
		deep.Equal(cmdWithArgs, expectedSecondCmdWithArgs) == nil {
		os.Exit(0)
	}

	os.Exit(1)
}

//nolint:paralleltest
func TestCreateManySnapshots_oneSnapshotOfManyErroredWithBookmarks(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	// simulate the zpool call used to detect bookmarks feature
	for _, v := range cmdWithArgs {
		if v == "feature@bookmarks" {
			fmt.Printf("tank\tfeature@bookmarks\tenabled\n") //nolint:forbidigo
			os.Exit(0)
		}
	}

	expectedCmdWithArgs := []string{
		"sh",
		"-c",
		"zfs snapshot pool/fs1@auto-2025-01-01 pool/fs2@auto-2025-01-01",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(0)
	}

	os.Exit(1)
}

//nolint:paralleltest
func TestCreateManySnapshots_oneSnapshotOfManyErroredWithoutBookmarks(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	// simulate the zpool call used to detect bookmarks feature - this time without bookmarks supported
	for _, v := range cmdWithArgs {
		if v == "feature@bookmarks" {
			os.Exit(0)
		}
	}

	expectedFirstCmdWithArgs := []string{
		"sh",
		"-c",
		"zfs snapshot pool/fs1@auto-2025-01-01",
	}

	if deep.Equal(cmdWithArgs, expectedFirstCmdWithArgs) == nil {
		os.Exit(0)
	}

	// second command will be `sh -c zfs snapshot pool/fs2@auto-2025-01-01`, which we let fail

	os.Exit(1)
}

//nolint:paralleltest
func Test_getArgMax_working(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"getconf",
		"ARG_MAX",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	fmt.Printf("123456\n") //nolint:forbidigo

	os.Exit(0)
}

//nolint:paralleltest
func Test_getArgMax_error(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"getconf",
		"ARG_MAX",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(0)
	}

	os.Exit(1)
}

//nolint:paralleltest
func Test_getArgMax_bogus(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"getconf",
		"ARG_MAX",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	fmt.Printf("bogus\n") //nolint:forbidigo

	os.Exit(0)
}

//nolint:paralleltest
func TestDestroySnapshot_working(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"zfs",
		"destroy",
		"-d",
		"pool1/fs1@snapshot1",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	os.Exit(0)
}

//nolint:paralleltest
func TestDestroySnapshot_error(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"zfs",
		"destroy",
		"-d",
		"pool1/fs1@snapshot1",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(0)
	}

	os.Exit(1)
}

//nolint:paralleltest
func TestDestroySnapshot_dryRun(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	os.Exit(1)
}
