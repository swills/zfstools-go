package zfs

import (
	"fmt"
	"os"
	"testing"

	"github.com/go-test/deep"

	"zfstools-go/pkg/zfstoolstest"
)

//nolint:paralleltest
func TestListDatasets(t *testing.T) {
	type args struct {
		pool       string
		properties []string
		debug      bool
	}

	//nolint:govet
	tests := []struct {
		name        string
		args        args
		mockCmdFunc string
		want        []Dataset
	}{
		{
			name: "twoDatasets",
			args: args{
				pool:       "",
				properties: []string{"mysql", "com.sun:auto-snapshot"},
				debug:      false,
			},
			mockCmdFunc: "TestListDatasets_EmptyPoolName",
			want: []Dataset{
				{
					Name: "pool/fs1",
					Properties: map[string]string{
						"type":  "filesystem",
						"mysql": "mysql",
					},
					DB: "",
				},
				{
					Name: "pool/fs2",
					Properties: map[string]string{
						"type":                  "filesystem",
						"com.sun:auto-snapshot": "true",
					},
					DB: "",
				},
			},
		},
		{
			name: "datasetsWithPoolName",
			args: args{
				pool:       "tank",
				properties: []string{"com.sun:auto-snapshot"},
				debug:      false,
			},
			mockCmdFunc: "TestListDatasets_PoolNameSet",
			want: []Dataset{
				{
					Name: "tank",
					Properties: map[string]string{
						"type": "filesystem",
					},
					DB: "",
				},
				{
					Name: "tank/ROOT",
					Properties: map[string]string{
						"type": "filesystem",
					},
					DB: "",
				},
				{
					Name: "tank/ROOT/default",
					Properties: map[string]string{
						"type":                  "filesystem",
						"com.sun:auto-snapshot": "true",
					},
					DB: "",
				},
			},
		},
		{
			name: "mySQLAndPostgreSQL",
			args: args{
				pool:       "dozer",
				properties: []string{"com.sun:auto-snapshot"},
				debug:      false,
			},
			mockCmdFunc: "TestListDatasets_MySQLAndPostgreSQL",
			want: []Dataset{
				{
					Name: "dozer",
					Properties: map[string]string{
						"type": "filesystem",
					},
					DB: "",
				},
				{
					Name: "dozer/mysql",
					Properties: map[string]string{
						"type":                  "filesystem",
						"com.sun:auto-snapshot": "mysql",
					},
					DB: "mysql",
				},
				{
					Name: "dozer/postgresql",
					Properties: map[string]string{
						"type":                  "filesystem",
						"com.sun:auto-snapshot": "postgresql",
					},
					DB: "postgresql",
				},
			},
		},
		{
			name: "shortLine",
			args: args{
				pool:       "",
				properties: []string{"mysql", "com.sun:auto-snapshot"},
				debug:      false,
			},
			mockCmdFunc: "TestListDatasets_EmptyPoolNameShortLine",
			want: []Dataset{
				{
					Name: "pool/fs1",
					Properties: map[string]string{
						"type":  "filesystem",
						"mysql": "mysql",
					},
					DB: "",
				},
				{
					Name: "pool/fs2",
					Properties: map[string]string{
						"type":                  "filesystem",
						"com.sun:auto-snapshot": "true",
					},
					DB: "",
				},
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			runZfsFn = zfstoolstest.MakeFakeCommand(testCase.mockCmdFunc)

			got := ListDatasets(testCase.args.pool, testCase.args.properties, testCase.args.debug)

			diff := deep.Equal(got, testCase.want)
			if diff != nil {
				t.Errorf("compare failed: %v", diff)
			}
		})
	}
}

func TestDataset_Equal(t *testing.T) {
	t.Parallel()

	datasetA := Dataset{Name: "tank/data"}
	datasetB := Dataset{Name: "tank/data"}
	datasetC := Dataset{Name: "tank/logs"}

	if !datasetA.Equals(datasetB) {
		t.Error("expected datasetA and datasetB to be equal")
	}

	if datasetA.Equals(datasetC) {
		t.Error("expected datasetA and datasetC to be different")
	}
}

// test helpers from here down

//nolint:paralleltest
func TestListDatasets_EmptyPoolName(_ *testing.T) {
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
		"name,type,mysql,com.sun:auto-snapshot",
		"-s",
		"name",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	fmt.Printf("pool/fs1\tfilesystem\tmysql\t-\npool/fs2\tfilesystem\t-\ttrue\n") //nolint:forbidigo

	os.Exit(0)
}

//nolint:paralleltest
func TestListDatasets_PoolNameSet(_ *testing.T) {
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
		"name,type,com.sun:auto-snapshot",
		"-s",
		"name",
		"-r",
		"tank",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	fmt.Printf("tank\tfilesystem\t-\ntank/ROOT\tfilesystem\t-\ntank/ROOT/default\tfilesystem\ttrue\n") //nolint:forbidigo

	os.Exit(0)
}

//nolint:paralleltest
func TestListDatasets_MySQLAndPostgreSQL(_ *testing.T) {
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
		"name,type,com.sun:auto-snapshot",
		"-s",
		"name",
		"-r",
		"dozer",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	fmt.Printf("dozer\tfilesystem\t-\ndozer/mysql\tfilesystem\tmysql\ndozer/postgresql\tfilesystem\tpostgresql\n") //nolint:forbidigo,lll

	os.Exit(0)
}

//nolint:paralleltest
func TestListDatasets_EmptyPoolNameShortLine(_ *testing.T) {
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
		"name,type,mysql,com.sun:auto-snapshot",
		"-s",
		"name",
	}

	if deep.Equal(cmdWithArgs, expectedCmdWithArgs) != nil {
		os.Exit(1)
	}

	fmt.Printf("bogus\npool/fs1\tfilesystem\tmysql\t-\npool/fs2\tfilesystem\t-\ttrue\n") //nolint:forbidigo

	os.Exit(0)
}
