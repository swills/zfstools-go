package zfs

import (
	"io"
	"testing"

	"github.com/go-test/deep"
)

func TestListPools(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pool     string
		props    []string
		output   string
		wantArgs []string
		want     []Pool
	}{
		{
			name: "all pools and properties",
			output: "tank\tsize\t9876543210987\ntank\thealth\tONLINE\n" +
				"dozer\tsize\t1234567901234\ndozer\thealth\tONLINE\n",
			wantArgs: []string{"get", "-H", "-p", "-o", "name,property,value", "all"},
			want: []Pool{
				{Name: "dozer", Properties: map[string]string{"size": "1234567901234", "health": "ONLINE"}},
				{Name: "tank", Properties: map[string]string{"size": "9876543210987", "health": "ONLINE"}},
			},
		},
		{
			name:     "named pool and properties",
			pool:     "tank",
			props:    []string{"health", "feature@bookmarks"},
			output:   "tank\thealth\tONLINE\ntank\tfeature@bookmarks\tenabled\n",
			wantArgs: []string{"get", "-H", "-p", "-o", "name,property,value", "health,feature@bookmarks", "tank"},
			want: []Pool{{Name: "tank", Properties: map[string]string{
				"health": "ONLINE", "feature@bookmarks": "enabled",
			}}},
		},
		{
			name:     "malformed row",
			props:    []string{"health"},
			output:   "incomplete_line",
			wantArgs: []string{"get", "-H", "-p", "-o", "name,property,value", "health"},
			want:     []Pool{},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			runner := &fakeRunner{output: []byte(testCase.output)}

			got, err := NewClient(runner, io.Discard).ListPools(t.Context(), testCase.pool, testCase.props, false)
			if err != nil {
				t.Fatalf("ListPools() error = %v", err)
			}

			if diff := deep.Equal(got, testCase.want); diff != nil {
				t.Errorf("pools differ: %v", diff)
			}

			wantCall := []commandCall{{name: "zpool", args: testCase.wantArgs}}
			if diff := deep.Equal(runner.calls, wantCall); diff != nil {
				t.Errorf("command differs: %v", diff)
			}
		})
	}
}

func TestListPoolsCommandError(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{err: errTestCommand}
	if _, err := NewClient(runner, io.Discard).ListPools(t.Context(), "", nil, false); err == nil {
		t.Fatal("ListPools() error = nil, want command error")
	}
}
