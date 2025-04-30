package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/pflag"

	"zfstools-go/pkg/config"
	"zfstools-go/pkg/zfstools"
)

func usage() {
	_, _ = fmt.Fprintf(os.Stderr, "Usage: %s [-dknpuv] <INTERVAL> <KEEP>\n", os.Args[0])
	_, _ = fmt.Fprintln(os.Stderr, "    -d              Show debug output.")
	_, _ = fmt.Fprintln(os.Stderr, "    -k              Keep zero-sized snapshots.")
	_, _ = fmt.Fprintln(os.Stderr, "    -n              Do a dry-run. Nothing is committed. Only show what would be done.")
	_, _ = fmt.Fprintln(os.Stderr, "    -p              Create snapshots in parallel.")
	_, _ = fmt.Fprintln(os.Stderr, "    -P pool         Act only on the specified pool.")
	_, _ = fmt.Fprintln(os.Stderr, "    -u              Use UTC for snapshots.")
	_, _ = fmt.Fprintln(os.Stderr, "    -v              Show what is being done.")
	_, _ = fmt.Fprintln(os.Stderr, "    INTERVAL        The interval to snapshot.")
	_, _ = fmt.Fprintln(os.Stderr, "    KEEP            How many snapshots to keep.")
	os.Exit(0)
}

func main() {
	var err error

	var pool string

	var keepZeroSized bool

	cfg := config.Config{
		Timestamp:              time.Now(),
		ShouldDestroyZeroSized: true,
	}

	pflag.BoolVarP(&cfg.UseUTC, "utc", "u", false, "")
	pflag.BoolVarP(&keepZeroSized, "keep-zero-sized-snapshots", "k", false, "")
	pflag.BoolVarP(&cfg.UseThreads, "parallel-snapshots", "p", false, "")
	pflag.StringVarP(&pool, "pool", "P", "", "")
	pflag.BoolVarP(&cfg.DryRun, "dry-run", "n", false, "")
	pflag.BoolVarP(&cfg.Verbose, "verbose", "v", false, "")
	pflag.BoolVarP(&cfg.Debug, "debug", "d", false, "")
	pflag.StringVarP(&cfg.SnapshotPrefix, "snapshot-prefix", "s", "zfs-auto-snap", "")
	pflag.Usage = usage
	pflag.Parse()

	if keepZeroSized {
		cfg.ShouldDestroyZeroSized = false
	}

	args := pflag.Args()
	if len(args) < 2 {
		usage()
	}

	cfg.Interval = args[0]

	cfg.Keep, err = strconv.Atoi(args[1])
	if err != nil {
		cfg.Keep = 0
	}

	datasets := zfstools.FindEligibleDatasets(cfg, pool)

	if cfg.Keep > 0 {
		zfstools.DoNewSnapshots(cfg, datasets)
	}

	zfstools.CleanupExpiredSnapshots(cfg, pool, datasets)
}
