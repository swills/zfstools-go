package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"zfstools-go/pkg/config"
	"zfstools-go/pkg/zfstools"
)

func usage() {
	_, _ = fmt.Fprintf(os.Stderr, "Usage: %s [-dknpuv] <INTERVAL> <KEEP>\n", os.Args[0])
	_, _ = fmt.Fprintln(os.Stderr, "    -d               Show debug output.")
	_, _ = fmt.Fprintln(os.Stderr, "    -k               Keep zero-sized snapshots.")
	_, _ = fmt.Fprintln(os.Stderr, "    -n               Dry-run. Show what would be done.")
	_, _ = fmt.Fprintln(os.Stderr, "    -p               Create snapshots in parallel.")
	_, _ = fmt.Fprintln(os.Stderr, "    -P pool          Act only on the specified pool.")
	_, _ = fmt.Fprintln(os.Stderr, "    -s prefix        Specify snapshot prefix.")
	_, _ = fmt.Fprintln(os.Stderr, "    -u               Use UTC for timestamps.")
	_, _ = fmt.Fprintln(os.Stderr, "    -v               Verbose output.")
	os.Exit(1)
}

func main() {
	cfg := config.Config{
		Timestamp: time.Now(),
	}

	var pool string

	flag.BoolVar(&cfg.Debug, "d", false, "")
	flag.BoolVar(&cfg.DestroyZeroSized, "k", false, "")
	flag.BoolVar(&cfg.DryRun, "n", false, "")
	flag.BoolVar(&cfg.UseThreads, "p", false, "")
	flag.StringVar(&pool, "P", "", "")
	flag.StringVar(&cfg.SnapshotPrefix, "s", "", "")
	flag.BoolVar(&cfg.UseUTC, "u", false, "")
	flag.BoolVar(&cfg.Verbose, "v", false, "")

	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	if len(args) < 2 {
		usage()
	}

	cfg.Interval = args[0]

	keep, err := strconv.Atoi(args[1])
	if err != nil || keep < 0 {
		_, _ = fmt.Fprintln(os.Stderr, "Invalid KEEP value")

		usage()
	}

	cfg.Keep = keep

	datasets := zfstools.FindEligibleDatasets(cfg, pool)

	if cfg.Keep > 0 {
		zfstools.DoNewSnapshots(cfg, datasets)
	}

	zfstools.CleanupExpiredSnapshots(cfg, pool, datasets)
}
