package zfs

import (
	"errors"
	"io"
	"testing"

	"github.com/go-test/deep"
)

var errTestReader = errors.New("test reader failed")

type errorReader struct {
	data []byte
}

func (reader *errorReader) Read(destination []byte) (int, error) {
	if len(reader.data) == 0 {
		return 0, errTestReader
	}

	count := copy(destination, reader.data)
	reader.data = reader.data[count:]

	return count, nil
}

func TestListDatasets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr    error
		name       string
		pool       string
		output     string
		properties []string
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
			wantErr: errInvalidDatasetOutput,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			runner := &fakeRunner{output: []byte(testCase.output)}

			got, err := NewClient(runner, io.Discard).ListDatasets(
				t.Context(), testCase.pool, testCase.properties, false,
			)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("ListDatasets() error = %v, want %v", err, testCase.wantErr)
			}

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

	runner := &fakeRunner{output: []byte("tank/data\tfilesystem\n"), err: errTestCommand}

	got, err := NewClient(runner, io.Discard).ListDatasets(t.Context(), "", nil, false)
	if !errors.Is(err, errTestCommand) {
		t.Fatalf("ListDatasets() error = %v, want command error", err)
	}

	if got != nil {
		t.Fatalf("ListDatasets() = %v, want nil result", got)
	}
}

func TestParseDatasetsReaderError(t *testing.T) {
	t.Parallel()

	reader := &errorReader{data: []byte("tank/data\tfilesystem\n")}

	got, err := parseDatasets(reader, nil)
	if !errors.Is(err, errTestReader) {
		t.Fatalf("parseDatasets() error = %v, want reader error", err)
	}

	if got != nil {
		t.Fatalf("parseDatasets() = %v, want nil result", got)
	}
}
