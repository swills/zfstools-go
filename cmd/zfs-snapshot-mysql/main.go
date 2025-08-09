package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
	_ "time/tzdata"

	"github.com/spf13/pflag"
)

var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

func FullVersion() string {
	return Version + " (commit " + Commit + ", built " + BuildDate + ")"
}

func usageWriter(writer io.Writer, name string) {
	_, _ = fmt.Fprintf(writer, "Usage: %s [-dnv] DATASET\n", name)
	_, _ = fmt.Fprintln(writer, "    -d              Show debug output.")
	_, _ = fmt.Fprintln(writer, "    -n              Do a dry-run. Nothing is committed. Only show what would be done.")
	_, _ = fmt.Fprintln(writer, "    -v              Show what is being done.")
}

func usage() {
	usageWriter(os.Stderr, os.Args[0])
	os.Exit(0)
}

func main() {
	var debug bool

	var dryRun bool

	var verbose bool

	pflag.BoolVarP(&debug, "debug", "d", false, "")
	pflag.BoolVarP(&dryRun, "dry-run", "n", false, "")
	pflag.BoolVarP(&verbose, "verbose", "v", false, "")
	pflag.Usage = usage
	showVersion := pflag.BoolP("version", "", false, "Print version information and exit")
	pflag.Parse()

	if *showVersion {
		fmt.Println(FullVersion())
		os.Exit(0)
	}

	if pflag.NArg() < 1 {
		usage()
	}

	dataset := pflag.Arg(0)
	timestamp := time.Now().Format("2006-01-02T15:04:05")
	snapshot := fmt.Sprintf("%s@%s", dataset, timestamp)

	// Command to be executed inside MySQL FLUSH lock
	zfsCmd := "zfs snapshot -r " + snapshot
	sql := fmt.Sprintf(`FLUSH LOGS; FLUSH TABLES WITH READ LOCK; SYSTEM %s; UNLOCK TABLES;`, zfsCmd)
	mysqlCmd := fmt.Sprintf(`mysql -e "%s"`, sql)

	if debug || verbose {
		fmt.Println(mysqlCmd) //nolint:forbidigo
	}

	if !dryRun {
		cmd := exec.Command("sh", "-c", mysqlCmd)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
	}
}
