package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/spf13/pflag"

	"zfstools-go/internal/config"
	"zfstools-go/internal/zfs"
	"zfstools-go/internal/zfstools"
)

var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

func usageWriter(writer io.Writer, name string) {
	_, _ = fmt.Fprintf(writer, "Usage: %s [-dnv]\n", name)
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

func version(writer io.Writer) {
	_, _ = fmt.Fprintf(writer, "%s (commit %s, built %s)", Version, Commit, BuildDate)

	os.Exit(0)
}

func main() {
	cfg := config.Config{
		Timestamp: time.Now(),
	}

	var pool string

	pflag.BoolVar(&cfg.Debug, "d", false, "")
	pflag.BoolVar(&cfg.DryRun, "n", false, "")
	pflag.BoolVar(&cfg.UseThreads, "p", false, "")
	pflag.StringVar(&pool, "P", "", "")
	pflag.BoolVar(&cfg.Verbose, "v", false, "")
	showVersion := pflag.BoolP("version", "", false, "Print version information and exit")
	pflag.Usage = usage
	pflag.Parse()

	if *showVersion {
		version(os.Stdout)
	}

	if len(pflag.Args()) > 0 {
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
