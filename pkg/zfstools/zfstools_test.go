package zfstools

import (
	"testing"

	"zfstools-go/pkg/config"
	"zfstools-go/pkg/zfs"
)

var createdSnapshots []string
var destroyedSnapshots []string

func init() {
	createdSnapshots = nil
	destroyedSnapshots = nil
	createManyFn = func(name string, datasets []zfs.Dataset, recursive, dryRun, verbose, debug, useThreads bool) {
		for _, ds := range datasets {
			createdSnapshots = append(createdSnapshots, ds.Name+"@"+name)
		}
	}
	destroySnapshotFn = func(name string, dryRun, debug bool) {
		destroyedSnapshots = append(destroyedSnapshots, name)
	}
}

func testConfig(interval string) config.Config {
	return config.Config{
		Interval: interval,
	}
}

func TestDoNewSnapshots(t *testing.T) {
	createdSnapshots = nil
	cfg := testConfig("frequent")
	datasets := map[string][]zfs.Dataset{
		"single":    {{Name: "pool/fs1"}},
		"recursive": {{Name: "pool/fs2"}},
	}
	DoNewSnapshots(cfg, datasets)

	if len(createdSnapshots) != 2 {
		t.Errorf("expected 2 snapshots, got %d", len(createdSnapshots))
	}
}

func TestGroupSnapshotsIntoDatasets(t *testing.T) {
	datasets := []zfs.Dataset{
		{Name: "pool/home"},
		{Name: "pool/data"},
	}
	snaps := []zfs.Snapshot{
		{Name: "pool/home@zfs-auto-snap_hourly-2025-01-01-01h00"},
		{Name: "pool/data@zfs-auto-snap_hourly-2025-01-01-01h00"},
	}
	grouped := GroupSnapshotsIntoDatasets(snaps, datasets)
	if len(grouped["pool/home"]) != 1 || len(grouped["pool/data"]) != 1 {
		t.Error("expected each dataset to have one snapshot")
	}
}
