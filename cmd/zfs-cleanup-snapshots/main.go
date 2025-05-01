package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"zfstools-go/pkg/config"
	"zfstools-go/pkg/zfs"
	"zfstools-go/pkg/zfstools"
)

func usageWriter(writer io.Writer, name string) {
	_, _ = fmt.Fprintf(writer, "Usage: %s [-dnv]", name)
	_, _ = fmt.Fprintln(writer, "    -d              Show debug output.")
	_, _ = fmt.Fprintln(writer, "    -n              Do a dry-run. Nothing is committed. Only show what would be done.")
	_, _ = fmt.Fprintln(writer, "    -p              Create snapshots in parallel.")
	_, _ = fmt.Fprintln(writer, "    -P pool         Act only on the specified pool.")
	_, _ = fmt.Fprintln(writer, "    -v              Show what is being done.")
}

func usage() {
	usageWriter(os.Stderr, os.Args[0])
	os.Exit(0)
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
