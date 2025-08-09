package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"time"
	_ "time/tzdata"

	"github.com/spf13/pflag"

	"zfstools-go/internal/config"
	"zfstools-go/internal/zfstools"
)

func usageWriter(writer io.Writer, name string) {
	_, _ = fmt.Fprintf(writer, "Usage: %s [-dknpuv] <INTERVAL> <KEEP>\n", name)
	_, _ = fmt.Fprintln(writer, "    -d              Show debug output.")
	_, _ = fmt.Fprintln(writer, "    -k              Keep zero-sized snapshots.")
	_, _ = fmt.Fprintln(writer, "    -n              Do a dry-run. Nothing is committed. Only show what would be done.")
	_, _ = fmt.Fprintln(writer, "    -p              Create snapshots in parallel.")
	_, _ = fmt.Fprintln(writer, "    -P pool         Act only on the specified pool.")
	_, _ = fmt.Fprintln(writer, "    -u              Use UTC for snapshots.")
	_, _ = fmt.Fprintln(writer, "    -v              Show what is being done.")
	_, _ = fmt.Fprintln(writer, "    INTERVAL        The interval to snapshot.")
	_, _ = fmt.Fprintln(writer, "    KEEP            How many snapshots to keep.")
}

func usage() {
	usageWriter(os.Stderr, os.Args[0])
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

	var keepInt int64

	keepInt, err = strconv.ParseInt(args[1], 10, 0)
	if err != nil {
		cfg.Keep = 0
	} else {
		cfg.Keep = int(keepInt)
	}

	datasets := zfstools.FindEligibleDatasets(cfg, pool)

	if cfg.Keep > 0 {
		zfstools.DoNewSnapshots(cfg, datasets)
	}

	zfstools.CleanupExpiredSnapshots(cfg, pool, datasets)
}
