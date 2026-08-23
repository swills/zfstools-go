package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"

	"zfstools-go/internal/config"
	"zfstools-go/internal/zfs"
	"zfstools-go/internal/zfstools"
)

const (
	autoSnapshotName = "zfs-auto-snapshot"
	cleanupName      = "zfs-cleanup-snapshots"
	mysqlName        = "zfs-snapshot-mysql"
)

// Run selects a command using the executable name, as required by a multi-call binary.
func Run(ctx context.Context, name string, args []string, stdout, stderr io.Writer, version, commit string) int {
	client := zfs.NewSystemClient(stdout)

	return run(ctx, name, args, stdout, stderr, version, commit, client)
}

func run(
	ctx context.Context,
	name string,
	args []string,
	stdout, stderr io.Writer,
	version, commit string,
	client zfs.Client,
) int {
	switch filepath.Base(name) {
	case autoSnapshotName:
		return runAutoSnapshot(ctx, name, args, stdout, stderr, version, commit, client)
	case cleanupName:
		return runCleanupSnapshots(ctx, name, args, stdout, stderr, version, commit, client)
	case mysqlName:
		return runSnapshotMySQL(ctx, name, args, stdout, stderr, version, commit, client)
	default:
		_, _ = fmt.Fprintf(stderr, "unknown command %q (expected %s, %s, or %s)\n",
			filepath.Base(name), autoSnapshotName, cleanupName, mysqlName)

		return 2
	}
}

func runAutoSnapshot(
	ctx context.Context,
	name string,
	args []string,
	stdout, stderr io.Writer,
	version, commit string,
	client zfs.Client,
) int {
	var pool string

	var keepZeroSized bool

	cfg := config.Config{
		Timestamp:              time.Now(),
		ShouldDestroyZeroSized: true,
	}
	flags := newFlagSet(name, stderr)
	flags.BoolVarP(&cfg.UseUTC, "utc", "u", false, "")
	flags.BoolVarP(&keepZeroSized, "keep-zero-sized-snapshots", "k", false, "")
	flags.BoolVarP(&cfg.UseThreads, "parallel-snapshots", "p", false, "")
	flags.StringVarP(&pool, "pool", "P", "", "")
	flags.BoolVarP(&cfg.DryRun, "dry-run", "n", false, "")
	flags.BoolVarP(&cfg.Verbose, "verbose", "v", false, "")
	flags.BoolVarP(&cfg.Debug, "debug", "d", false, "")
	flags.StringVarP(&cfg.SnapshotPrefix, "snapshot-prefix", "s", "zfs-auto-snap", "")
	showVersion := flags.BoolP("version", "", false, "Print version information and exit")
	flags.Usage = func() { writeAutoSnapshotUsage(stderr, name) }

	if code := parse(flags, args); code != -1 {
		return code
	}

	if *showVersion {
		writeVersion(stdout, version, commit)

		return 0
	}

	if keepZeroSized {
		cfg.ShouldDestroyZeroSized = false
	}

	if flags.NArg() < 2 {
		flags.Usage()

		return 0
	}

	cfg.Interval = flags.Arg(0)

	keep, err := parseKeep(flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "invalid KEEP %q: must be a non-negative decimal integer\n", flags.Arg(1))

		return 2
	}

	cfg.Keep = keep

	tools := zfstools.New(client, stdout)

	datasets := tools.FindEligibleDatasets(ctx, cfg, pool)

	var createErr error
	if cfg.Keep > 0 {
		createErr = tools.DoNewSnapshots(ctx, cfg, datasets)
	}

	tools.CleanupExpiredSnapshots(ctx, cfg, pool, datasets)

	if createErr != nil {
		_, _ = fmt.Fprintf(stderr, "Error creating snapshots: %v\n", createErr)

		return 1
	}

	return 0
}

func parseKeep(value string) (int, error) {
	if value == "" {
		return 0, strconv.ErrSyntax
	}

	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, strconv.ErrSyntax
		}
	}

	keep, err := strconv.ParseInt(value, 10, 0)
	if err != nil {
		return 0, fmt.Errorf("parse KEEP: %w", err)
	}

	return int(keep), nil
}

func runCleanupSnapshots(
	ctx context.Context,
	name string,
	args []string,
	stdout, stderr io.Writer,
	version, commit string,
	client zfs.Client,
) int {
	cfg := config.Config{Timestamp: time.Now()}

	var pool string

	flags := newFlagSet(name, stderr)
	flags.BoolVarP(&cfg.Debug, "debug", "d", false, "")
	flags.BoolVarP(&cfg.DryRun, "dry-run", "n", false, "")
	flags.BoolVarP(&cfg.UseThreads, "parallel-snapshots", "p", false, "")
	flags.StringVarP(&pool, "pool", "P", "", "")
	flags.BoolVarP(&cfg.Verbose, "verbose", "v", false, "")
	showVersion := flags.BoolP("version", "", false, "Print version information and exit")
	flags.Usage = func() { writeCleanupUsage(stderr, name) }

	if code := parse(flags, args); code != -1 {
		return code
	}

	if *showVersion {
		writeVersion(stdout, version, commit)

		return 0
	}

	if flags.NArg() > 0 {
		flags.Usage()

		return 0
	}

	tools := zfstools.New(client, stdout)

	snapshots, err := client.ListSnapshots(ctx, pool, true, cfg.Debug)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error listing snapshots: %v\n", err)

		return 1
	}

	filtered := make([]zfs.Snapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if !strings.Contains(snapshot.Name, "zfs-auto-snap_") && snapshot.IsZero(ctx, cfg.Debug) {
			filtered = append(filtered, snapshot)
		}
	}

	datasets := client.ListDatasets(ctx, pool, []string{}, cfg.Debug)
	grouped := zfstools.GroupSnapshotsIntoDatasets(filtered, datasets)
	tools.DatasetsDestroyZeroSizedSnapshots(ctx, grouped, cfg)

	return 0
}

func runSnapshotMySQL(
	ctx context.Context,
	name string,
	args []string,
	stdout, stderr io.Writer,
	version, commit string,
	client zfs.Client,
) int {
	var debug, dryRun, verbose bool

	flags := newFlagSet(name, stderr)
	flags.BoolVarP(&debug, "debug", "d", false, "")
	flags.BoolVarP(&dryRun, "dry-run", "n", false, "")
	flags.BoolVarP(&verbose, "verbose", "v", false, "")
	showVersion := flags.BoolP("version", "", false, "Print version information and exit")
	flags.Usage = func() { writeSnapshotMySQLUsage(stderr, name) }

	if code := parse(flags, args); code != -1 {
		return code
	}

	if *showVersion {
		writeVersion(stdout, version, commit)

		return 0
	}

	if flags.NArg() < 1 {
		flags.Usage()

		return 0
	}

	err := client.CreateSnapshots(
		ctx,
		[]string{flags.Arg(0)}, time.Now().Format("2006-01-02T15:04:05"), true, "mysql",
		dryRun, verbose, debug,
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error creating snapshot: %v\n", err)

		return 1
	}

	return 0
}

func newFlagSet(name string, stderr io.Writer) *pflag.FlagSet {
	flags := pflag.NewFlagSet(name, pflag.ContinueOnError)
	flags.SetOutput(stderr)

	return flags
}

func parse(flags *pflag.FlagSet, args []string) int {
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return 0
		}

		_, _ = fmt.Fprintln(flags.Output(), err)
		flags.Usage()

		return 2
	}

	return -1
}

func writeVersion(writer io.Writer, version, commit string) {
	_, _ = fmt.Fprintf(writer, "%s (commit %s)\n", version, commit)
}

func writeAutoSnapshotUsage(writer io.Writer, name string) {
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

func writeCleanupUsage(writer io.Writer, name string) {
	_, _ = fmt.Fprintf(writer, "Usage: %s [-dnv]\n", name)
	_, _ = fmt.Fprintln(writer, "    -d              Show debug output.")
	_, _ = fmt.Fprintln(writer, "    -n              Do a dry-run. Nothing is committed. Only show what would be done.")
	_, _ = fmt.Fprintln(writer, "    -p              Create snapshots in parallel.")
	_, _ = fmt.Fprintln(writer, "    -P pool         Act only on the specified pool.")
	_, _ = fmt.Fprintln(writer, "    -v              Show what is being done.")
}

func writeSnapshotMySQLUsage(writer io.Writer, name string) {
	_, _ = fmt.Fprintf(writer, "Usage: %s [-dnv] DATASET\n", name)
	_, _ = fmt.Fprintln(writer, "    -d              Show debug output.")
	_, _ = fmt.Fprintln(writer, "    -n              Do a dry-run. Nothing is committed. Only show what would be done.")
	_, _ = fmt.Fprintln(writer, "    -v              Show what is being done.")
}
