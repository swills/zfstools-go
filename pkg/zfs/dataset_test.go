package zfs

import (
	"fmt"
	"github.com/go-test/deep"
	"os"
	"testing"
	"zfstools-go/pkg/zfstoolstest"
)

func TestListDatasets(t *testing.T) {
	type args struct {
		pool       string
		properties []string
		debug      bool
	}

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
	a := Dataset{Name: "tank/data"}
	b := Dataset{Name: "tank/data"}
	c := Dataset{Name: "tank/logs"}

	if !a.Equals(b) {
		t.Error("expected a and b to be equal")
	}
	if a.Equals(c) {
		t.Error("expected a and c to be different")
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

	fmt.Printf("pool/fs1\tfilesystem\tmysql\t-\npool/fs2\tfilesystem\t-\ttrue\n")

	os.Exit(0)
}
