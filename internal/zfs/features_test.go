package zfs

import (
	"io"
	"testing"
)

func TestHasBookmarks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err    error
		name   string
		output string
		want   bool
	}{
		{
			name: "supported", output: "pool\tfeature@bookmarks\tenabled\n", want: true,
		},
		{
			name: "unsupported", output: "pool\tother-feature\tenabled\n",
		},
		{
			name: "error", err: errTestCommand,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			client := NewClient(&fakeRunner{output: []byte(testCase.output), err: testCase.err}, io.Discard)
			if got := client.hasBookmarks(false); got != testCase.want {
				t.Errorf("hasBookmarks() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestHasBookmarksCachesResult(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{output: []byte("pool\tfeature@bookmarks\tenabled\n")}
	client := NewClient(runner, io.Discard)

	client.hasBookmarks(false)
	client.hasBookmarks(false)

	if got := len(runner.calls); got != 1 {
		t.Errorf("ListPools() call count = %d, want 1", got)
	}
}
