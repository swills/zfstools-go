package main

import (
	"os"
	_ "time/tzdata"

	"zfstools-go/internal/cli"
)

var (
	Version = "dev"
	Commit  = "none"
)

func main() {
	os.Exit(cli.RunAutoSnapshot(os.Args[0], os.Args[1:], os.Stdout, os.Stderr, Version, Commit))
}
