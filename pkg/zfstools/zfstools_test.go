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
	createManySnapshotsFn = func(name string, datasets []zfs.Dataset, _, _, _, _, _ bool) error {
		for _, ds := range datasets {
			createdSnapshots = append(createdSnapshots, ds.Name+"@"+name)
		}

		return nil
	}
	destroySnapshotFn = func(name string, _, _ bool) {
		destroyedSnapshots = append(destroyedSnapshots, name)
	}
}

func testConfig(interval string) config.Config {
	return config.Config{
		Interval: interval,
	}
}

func TestDoNewSnapshots(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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
