package zfs

import (
	"os/exec"
	"testing"
)

func TestListDatasets_ParsesCorrectly(t *testing.T) {
	runZfsFn = func(name string, args ...string) *exec.Cmd {
		return exec.Command("echo", "pool/fs1	filesystem	mysql	-\npool/fs2	filesystem	-	true\n")
	}

	datasets := ListDatasets("", []string{"mysql", "com.sun:auto-snapshot"}, false)
	if len(datasets) != 2 {
		t.Fatalf("expected 2 datasets, got %d", len(datasets))
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
