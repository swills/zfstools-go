package zfs

import (
	"fmt"
	"os"
	"testing"

	"github.com/go-test/deep"

	"zfstools-go/internal/zfstoolstest"
)

//nolint:paralleltest
func TestListPools(t *testing.T) {
	type args struct {
		name     string
		cmdProps []string
		debug    bool
	}

	tests := []struct {
		name        string
		mockCmdFunc string
		want        []Pool
		args        args
		wantErr     bool
	}{
		{
			name: "allPoolsAllProps",
			args: args{
				name:     "",
				cmdProps: nil,
				debug:    false,
			},
			mockCmdFunc: "TestListPools_allPoolsAllProps",
			want: []Pool{
				{
					Name: "dozer",
					Properties: map[string]string{
						"size":     "1234567901234",
						"capacity": "321",
						"altroot":  "-",
						"health":   "ONLINE",
					},
				},
				{
					Name: "tank",
					Properties: map[string]string{
						"size":     "9876543210987",
						"capacity": "123",
						"altroot":  "-",
						"health":   "ONLINE",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "onePoolAllProps",
			args: args{
				name:     "tank",
				cmdProps: nil,
				debug:    false,
			},
			mockCmdFunc: "TestListPools_onePoolAllProps",
			want: []Pool{
				{
					Name: "tank",
					Properties: map[string]string{
						"size":     "9959959846912",
						"capacity": "68",
						"altroot":  "-",
						"health":   "ONLINE",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "allPoolsOneProp",
			args: args{
				name:     "",
				cmdProps: []string{"health"},
				debug:    false,
			},
			mockCmdFunc: "TestListPools_allPoolsOneProp",
			want: []Pool{
				{
					Name: "dozer",
					Properties: map[string]string{
						"health": "ONLINE",
					},
				},
				{
					Name: "tank",
					Properties: map[string]string{
						"health": "ONLINE",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "onePoolTwoProps",
			args: args{
				name:     "tank",
				cmdProps: []string{"health", "feature@bookmarks"},
				debug:    false,
			},
			mockCmdFunc: "TestListPools_onePoolTwoProps",
			want: []Pool{
				{
					Name: "tank",
					Properties: map[string]string{
						"health":            "ONLINE",
						"feature@bookmarks": "enabled",
					},
				},
			},
			wantErr: false,
		},
		{
			name:        "invalidLine",
			args:        args{"", []string{"feature@bookmarks"}, false},
			mockCmdFunc: "TestListPools_invalidLine",
			want:        []Pool{},
			wantErr:     false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			runZpoolFn = zfstoolstest.MakeFakeCommand(testCase.mockCmdFunc)

			got, err := ListPools(testCase.args.name, testCase.args.cmdProps, testCase.args.debug)
			if (err != nil) != testCase.wantErr {
				t.Errorf("ListPools() error = %v, wantErr %v", err, testCase.wantErr)

				return
			}

			diff := deep.Equal(got, testCase.want)
			if diff != nil {
				t.Errorf("compare failed: %#v", diff)
			}
		})
	}
}

// test helpers from here down

//nolint:paralleltest
func TestListPools_allPoolsAllProps(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"zpool",
		"get",
		"-H",
		"-p",
		"-o",
		"name,property,value",
		"all",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	//nolint:forbidigo
	fmt.Printf(`tank	size	9876543210987
tank	capacity	123
tank	altroot	-
tank	health	ONLINE
`)

	//nolint:forbidigo
	fmt.Printf(`dozer	size	1234567901234
dozer	capacity	321
dozer	altroot	-
dozer	health	ONLINE
`)

	os.Exit(0)
}

//nolint:paralleltest
func TestListPools_onePoolAllProps(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"zpool",
		"get",
		"-H",
		"-p",
		"-o",
		"name,property,value",
		"all",
		"tank",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	//nolint:forbidigo
	fmt.Printf(`tank	size	9959959846912
tank	capacity	68
tank	altroot	-
tank	health	ONLINE
`)

	os.Exit(0)
}

//nolint:paralleltest
func TestListPools_allPoolsOneProp(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"zpool",
		"get",
		"-H",
		"-p",
		"-o",
		"name,property,value",
		"health",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	//nolint:forbidigo
	fmt.Printf(`tank	health	ONLINE
`)

	//nolint:forbidigo
	fmt.Printf(`dozer	health	ONLINE
`)

	os.Exit(0)
}

//nolint:paralleltest
func TestListPools_onePoolTwoProps(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"zpool",
		"get",
		"-H",
		"-p",
		"-o",
		"name,property,value",
		"health,feature@bookmarks",
		"tank",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	//nolint:forbidigo
	fmt.Printf(`tank	health	ONLINE
tank	feature@bookmarks	enabled
`)

	os.Exit(0)
}

//nolint:paralleltest
func TestListPools_invalidLine(_ *testing.T) {
	if !zfstoolstest.IsTestEnv() {
		return
	}

	cmdWithArgs := os.Args[3:]

	expectedCmdWithArgs := []string{
		"zpool",
		"get",
		"-H",
		"-p",
		"-o",
		"name,property,value",
		"feature@bookmarks",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	fmt.Printf(`incomplete_line`) //nolint:forbidigo

	os.Exit(0)
}
