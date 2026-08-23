package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	_ "time/tzdata"

	"zfstools-go/internal/cli"
)

var (
	Version = "dev"
	Commit  = "none"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Run(ctx, os.Args[0], os.Args[1:], os.Stdout, os.Stderr, Version, Commit)

	stop()

	os.Exit(code)
}
