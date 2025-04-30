package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [-dnv] DATASET\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "    -d               Show debug output.")
	fmt.Fprintln(os.Stderr, "    -n               Dry-run. Do not execute any commands.")
	fmt.Fprintln(os.Stderr, "    -v               Show what is being done.")
	os.Exit(1)
}

func main() {
	var debug bool

	var verbose bool

	var dryRun bool

	flag.BoolVar(&debug, "d", false, "")
	flag.BoolVar(&dryRun, "n", false, "")
	flag.BoolVar(&verbose, "v", false, "")
	flag.Usage = usage
	flag.Parse()

	if flag.NArg() < 1 {
		usage()
	}

	dataset := flag.Arg(0)
	timestamp := time.Now().Format("2006-01-02T15:04:05")
	snapshot := fmt.Sprintf("%s@%s", dataset, timestamp)

	// Command to be executed inside MySQL FLUSH lock
	zfsCmd := "zfs snapshot -r " + snapshot
	sql := fmt.Sprintf(`FLUSH LOGS; FLUSH TABLES WITH READ LOCK; SYSTEM %s; UNLOCK TABLES;`, zfsCmd)
	mysqlCmd := fmt.Sprintf(`mysql -e "%s"`, sql)

	if debug || verbose {
		fmt.Println(mysqlCmd)
	}

	if !dryRun {
		cmd := exec.Command("sh", "-c", mysqlCmd)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
	}
}
