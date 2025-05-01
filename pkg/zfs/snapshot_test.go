package zfs

import (
	"os/exec"
	"testing"
)

var _ = exec.Command

//nolint:paralleltest
func TestGetUsed_Stale(t *testing.T) {
	staleSnapshotSize = true
	runZfsFn = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command("echo", "4096")
	}

	snap := Snapshot{Name: "pool/fs@snap", Used: 2048}
	used := snap.GetUsed(true)

	if used != 4096 {
		t.Errorf("expected 4096, got %d", used)
	}
}

//nolint:paralleltest
func TestGetUsed_NotStale(t *testing.T) {
	staleSnapshotSize = false
	snap := Snapshot{Name: "pool/fs@snap", Used: 1024}

	if snap.GetUsed(false) != 1024 {
		t.Error("expected GetUsed to return original Used value")
	}
}

//nolint:paralleltest
func TestIsZero(t *testing.T) {
	staleSnapshotSize = false
	runZfsFn = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command("")
	}

	snap := Snapshot{Name: "pool/fs@snap", Used: 0}
	if !snap.IsZero(false) {
		t.Error("expected IsZero to return true for Used=0")
	}

	snap.Used = 123
	if snap.IsZero(false) {
		t.Error("expected IsZero to return false for Used=123")
	}
}

//nolint:paralleltest
func TestDestroySnapshot_DryRun(t *testing.T) {
	var ran bool

	runZfsFn = func(_ string, _ ...string) *exec.Cmd {
		ran = true

		return exec.Command("false")
	}

	staleSnapshotSize = false
	DestroySnapshot("pool/fs@snap", true, false)

	if ran {
		t.Error("expected no command to run in dry-run mode")
	}
}

//nolint:paralleltest
func TestDestroySnapshot_Real(t *testing.T) {
	runZfsFn = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command("echo")
	}

	staleSnapshotSize = false
	DestroySnapshot("pool/fs@snap", false, false)

	if !staleSnapshotSize {
		t.Error("expected staleSnapshotSize = true after successful destroy")
	}
}

//nolint:paralleltest
func TestListSnapshots(t *testing.T) {
	runZfsFn = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command("echo", "pool/fs@a\t1024\n"+
			"pool/fs@b\t0")
	}

	snaps, err := ListSnapshotsFn("", false, false)
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}

	if len(snaps) != 2 {
		t.Errorf("expected 2 snapshots, got %d", len(snaps))
	}

	if snaps[0].Used != 1024 {
		t.Errorf("expected Used=1024 for first snapshot, got %d", snaps[0].Used)
	}
}

//nolint:paralleltest
func TestCreate(t *testing.T) {
	var ran bool

	runZfsFn = func(_ string, _ ...string) *exec.Cmd {
		ran = true

		return exec.Command("echo")
	}

	Create([]string{"pool/fs@snap"}, false, "", false, true, true)

	if !ran {
		t.Error("expected zfs snapshot to run")
	}
}

//nolint:paralleltest
func TestCreateMany(t *testing.T) {
	count := 0
	runZfsFn = func(_ string, _ ...string) *exec.Cmd {
		count++

		return exec.Command("echo")
	}

	CreateMany("auto-2025-01-01", []Dataset{
		{Name: "pool/fs@a"},
		{Name: "pool/fs@b"},
	}, false, false, true, true, false)

	if count != 2 {
		t.Errorf("expected 2 snapshots to be created, got %d", count)
	}
}
