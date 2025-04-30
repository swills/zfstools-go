package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	flag "github.com/spf13/pflag"
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
	cfg := config.Config{
		Timestamp: time.Now(),
	}

	var pool string

	flag.BoolVarP(&cfg.Debug, "debug", "d", false, "")
	flag.BoolVarP(&cfg.DestroyZeroSized, "keep-zeros", "k", false, "")
	flag.BoolVarP(&cfg.DryRun, "dry-run", "n", false, "")
	flag.BoolVarP(&cfg.UseThreads, "parallel", "p", false, "")
	flag.StringVarP(&pool, "pool", "P", "", "")
	flag.BoolVarP(&cfg.UseUTC, "utc", "u", false, "")
	flag.BoolVarP(&cfg.Verbose, "verbose", "v", false, "")
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
