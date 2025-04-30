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
	fmt.Fprintf(os.Stderr, "Usage: %s [-dnpv] [-P pool]\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "    -d               Show debug output.")
	fmt.Fprintln(os.Stderr, "    -n               Dry-run. Show what would be done.")
	fmt.Fprintln(os.Stderr, "    -p               Destroy snapshots in parallel.")
	fmt.Fprintln(os.Stderr, "    -P pool          Act only on the specified pool.")
	fmt.Fprintln(os.Stderr, "    -v               Verbose output.")
	os.Exit(1)
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
	snapshots, err := zfs.ListSnapshots(pool, true, cfg.Debug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing snapshots: %v\n", err)
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
	datasets, err := zfs.ListDatasets(pool, []string{}, cfg.Debug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing datasets: %v\n", err)
		os.Exit(1)
	}

	// Group and destroy
	grouped := zfstools.GroupSnapshotsIntoDatasets(filtered, datasets)
	zfstools.DatasetsDestroyZeroSizedSnapshots(grouped, cfg)
}
