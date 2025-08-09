package config

import "time"

type Config struct {
	Timestamp              time.Time
	Interval               string
	SnapshotPrefix         string
	Keep                   int
	UseUTC                 bool
	Verbose                bool
	Debug                  bool
	DryRun                 bool
	UseThreads             bool
	ShouldDestroyZeroSized bool
}
