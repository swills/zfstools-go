package zfs

import (
	"io"
	"testing"

	"github.com/go-test/deep"
)

func TestListDatasets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		pool       string
		properties []string
		output     string
		wantArgs   []string
		want       []Dataset
	}{
		{
			name:       "all pools",
			properties: []string{"mysql", "com.sun:auto-snapshot"},
			output:     "pool/fs1\tfilesystem\tmysql\t-\npool/fs2\tfilesystem\t-\ttrue\n",
			wantArgs: []string{
				"list", "-H", "-t", "filesystem,volume", "-o",
				"name,type,mysql,com.sun:auto-snapshot", "-s", "name",
			},
			want: []Dataset{
				{Name: "pool/fs1", Properties: map[string]string{"type": "filesystem", "mysql": "mysql"}},
				{Name: "pool/fs2", Properties: map[string]string{
					"type": "filesystem", "com.sun:auto-snapshot": "true",
				}},
			},
		},
		{
			name:       "pool and database properties",
			pool:       "dozer",
			properties: []string{"com.sun:auto-snapshot"},
			output: "dozer\tfilesystem\t-\n" +
				"dozer/mysql\tfilesystem\tmysql\n" +
				"dozer/postgresql\tfilesystem\tpostgresql\n",
			wantArgs: []string{
				"list", "-H", "-t", "filesystem,volume", "-o",
				"name,type,com.sun:auto-snapshot", "-s", "name", "-r", "dozer",
			},
			want: []Dataset{
				{Name: "dozer", Properties: map[string]string{"type": "filesystem"}},
				{Name: "dozer/mysql", Properties: map[string]string{
					"type": "filesystem", "com.sun:auto-snapshot": "mysql",
				}, DB: "mysql"},
				{Name: "dozer/postgresql", Properties: map[string]string{
					"type": "filesystem", "com.sun:auto-snapshot": "postgresql",
				}, DB: "postgresql"},
			},
		},
		{
			name:       "malformed rows",
			properties: []string{"com.sun:auto-snapshot"},
			output:     "bogus\nmissing\tfilesystem\nvalid\tfilesystem\ttrue\n",
			wantArgs: []string{
				"list", "-H", "-t", "filesystem,volume", "-o",
				"name,type,com.sun:auto-snapshot", "-s", "name",
			},
			want: []Dataset{{Name: "valid", Properties: map[string]string{
				"type": "filesystem", "com.sun:auto-snapshot": "true",
			}}},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			runner := &fakeRunner{output: []byte(testCase.output)}
			got := NewClient(runner, io.Discard).ListDatasets(t.Context(), testCase.pool, testCase.properties, false)

			if diff := deep.Equal(got, testCase.want); diff != nil {
				t.Errorf("datasets differ: %v", diff)
			}

			wantCall := []commandCall{{name: "zfs", args: testCase.wantArgs}}
			if diff := deep.Equal(runner.calls, wantCall); diff != nil {
				t.Errorf("command differs: %v", diff)
			}
		})
	}
}

func TestListDatasetsCommandError(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{err: errTestCommand}
	if got := NewClient(runner, io.Discard).ListDatasets(t.Context(), "", nil, false); len(got) != 0 {
		t.Fatalf("ListDatasets() = %v, want empty result", got)
	}
}
