package zfs

import (
	"os/exec"
	"testing"
)

func TestListPools_SingleProperty(t *testing.T) {
	t.Parallel()

	runZpoolFn = func(_ string, _ ...string) *exec.Cmd {
		return fakeCmd("testpool\tfeature@bookmarks\tenabled")
	}

	pools, err := ListPools("", []string{"feature@bookmarks"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pools) != 1 {
		t.Fatalf("expected 1 pool, got %d", len(pools))
	}

	p := pools[0]
	if p.Properties["feature@bookmarks"] != "enabled" {
		t.Fatalf("expected feature@bookmarks=enabled, got %q", p.Properties["feature@bookmarks"])
	}
}

func TestListPools_InvalidLine(t *testing.T) {
	t.Parallel()

	runZpoolFn = func(_ string, _ ...string) *exec.Cmd {
		return fakeCmd("incomplete_line")
	}

	pools, err := ListPools("", []string{"feature@bookmarks"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pools) != 0 {
		t.Fatalf("expected 0 pools, got %d", len(pools))
	}
}

func fakeCmd(output string) *exec.Cmd {
	return exec.Command("echo", output)
}
