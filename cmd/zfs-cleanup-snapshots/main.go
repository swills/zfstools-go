package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"zfstools-go/pkg/config"
	"zfstools-go/pkg/zfs"
	"zfstools-go/pkg/zfstools"
)

func usage() {
	_, _ = fmt.Fprintln(os.Stderr, "Usage: /usr/local/sbin/zfs-cleanup-snapshots [-dnv]")
	_, _ = fmt.Fprintln(os.Stderr, "    -d              Show debug output.")
	_, _ = fmt.Fprintln(os.Stderr, "    -n              Do a dry-run. Nothing is committed. Only show what would be done.")
	_, _ = fmt.Fprintln(os.Stderr, "    -p              Create snapshots in parallel.")
	_, _ = fmt.Fprintln(os.Stderr, "    -P pool         Act only on the specified pool.")
	_, _ = fmt.Fprintln(os.Stderr, "    -v              Show what is being done.")
}

func main() {
	cfg := config.Config{
		Timestamp: time.Now(),
	}

	var pool string

	flag.BoolVar(&cfg.Debug, "d", false, "")
	flag.BoolVar(&cfg.DryRun, "n", false, "")
	flag.BoolVar(&cfg.UseThreads, "p", false, "")
	flag.StringVar(&pool, "P", "", "")
	flag.BoolVar(&cfg.Verbose, "v", false, "")

	flag.Usage = usage
	flag.Parse()

	if len(flag.Args()) > 0 {
		usage()
	}

	// List all snapshots recursively
	snapshots, err := zfs.ListSnapshotsFn(pool, true, cfg.Debug)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error listing snapshots: %v\n", err)
		os.Exit(1)
	}

	// Filter snapshots that are zero-sized and not created by zfs-auto-snapshot
	var filtered []zfs.Snapshot

	prefix := "zfs-auto-snap_"

	for _, snap := range snapshots {
		if !strings.Contains(snap.Name, prefix) && snap.IsZero(cfg.Debug) {
			filtered = append(filtered, snap)
		}
	}

	// Get dataset list
	datasets := zfs.ListDatasets(pool, []string{}, cfg.Debug)

	// Group and destroy
	grouped := zfstools.GroupSnapshotsIntoDatasets(filtered, datasets)
	zfstools.DatasetsDestroyZeroSizedSnapshots(grouped, cfg)
}
