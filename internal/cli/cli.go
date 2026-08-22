package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
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
func Run(name string, args []string, stdout, stderr io.Writer, version, commit string) int {
	switch filepath.Base(name) {
	case autoSnapshotName:
		return RunAutoSnapshot(name, args, stdout, stderr, version, commit)
	case cleanupName:
		return RunCleanupSnapshots(name, args, stdout, stderr, version, commit)
	case mysqlName:
		return RunSnapshotMySQL(name, args, stdout, stderr, version, commit)
	default:
		_, _ = fmt.Fprintf(stderr, "unknown command %q (expected %s, %s, or %s)\n",
			filepath.Base(name), autoSnapshotName, cleanupName, mysqlName)

		return 2
	}
}

// RunAutoSnapshot runs zfs-auto-snapshot.
func RunAutoSnapshot(name string, args []string, stdout, stderr io.Writer, version, commit string) int {
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

	keep, err := strconv.ParseInt(flags.Arg(1), 10, 0)
	if err == nil {
		cfg.Keep = int(keep)
	}

	datasets := zfstools.FindEligibleDatasets(cfg, pool)
	if cfg.Keep > 0 {
		zfstools.DoNewSnapshots(cfg, datasets)
	}

	zfstools.CleanupExpiredSnapshots(cfg, pool, datasets)

	return 0
}

// RunCleanupSnapshots runs zfs-cleanup-snapshots.
func RunCleanupSnapshots(name string, args []string, stdout, stderr io.Writer, version, commit string) int {
	cfg := config.Config{Timestamp: time.Now()}

	var pool string

	flags := newFlagSet(name, stderr)
	flags.BoolVar(&cfg.Debug, "d", false, "")
	flags.BoolVar(&cfg.DryRun, "n", false, "")
	flags.BoolVar(&cfg.UseThreads, "p", false, "")
	flags.StringVar(&pool, "P", "", "")
	flags.BoolVar(&cfg.Verbose, "v", false, "")
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

	snapshots, err := zfs.ListSnapshotsFn(pool, true, cfg.Debug)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error listing snapshots: %v\n", err)

		return 1
	}

	filtered := make([]zfs.Snapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if !strings.Contains(snapshot.Name, "zfs-auto-snap_") && snapshot.IsZero(cfg.Debug) {
			filtered = append(filtered, snapshot)
		}
	}

	datasets := zfs.ListDatasets(pool, []string{}, cfg.Debug)
	grouped := zfstools.GroupSnapshotsIntoDatasets(filtered, datasets)
	zfstools.DatasetsDestroyZeroSizedSnapshots(grouped, cfg)

	return 0
}

// RunSnapshotMySQL runs zfs-snapshot-mysql.
func RunSnapshotMySQL(name string, args []string, stdout, stderr io.Writer, version, commit string) int {
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

	snapshot := fmt.Sprintf("%s@%s", flags.Arg(0), time.Now().Format("2006-01-02T15:04:05"))
	zfsCmd := "zfs snapshot -r " + snapshot
	sql := fmt.Sprintf(`FLUSH LOGS; FLUSH TABLES WITH READ LOCK; SYSTEM %s; UNLOCK TABLES;`, zfsCmd)
	mysqlCmd := fmt.Sprintf(`mysql -e "%s"`, sql)

	if debug || verbose {
		_, _ = fmt.Fprintln(stdout, mysqlCmd)
	}

	if !dryRun {
		cmd := exec.CommandContext(context.Background(), "sh", "-c", mysqlCmd)
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		_ = cmd.Run()
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
