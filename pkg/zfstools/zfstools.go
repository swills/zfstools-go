package zfstools

import (
	"fmt"
	"strings"
	"sync"

	"zfstools-go/pkg/config"
	"zfstools-go/pkg/zfs"
)

func snapshotProperty() string {
	return "com.sun:auto-snapshot"
}

func snapshotPrefix(cfg config.Config) string {
	if cfg.SnapshotPrefix != "" {
		return cfg.SnapshotPrefix
	}

	return "zfs-auto-snap"
}

func snapshotPrefixInterval(cfg config.Config) string {
	return snapshotPrefix(cfg) + "_" + cfg.Interval + "-"
}

func snapshotFormat() string {
	return "2006-01-02-15h04"
}

func snapshotName(cfg config.Config) string {
	timestamp := cfg.Timestamp
	if cfg.UseUTC {
		timestamp = timestamp.UTC()

		return snapshotPrefixInterval(cfg) + timestamp.Format(snapshotFormat()) + "U"
	}

	return snapshotPrefixInterval(cfg) + timestamp.Format(snapshotFormat())
}

func filterDatasets(datasets []zfs.Dataset, included, excluded *[]zfs.Dataset, prop string) {
	all := append([]zfs.Dataset{}, *included...)
	all = append(all, *excluded...)

	for _, dataset := range datasets {
		// skip if already included or excluded
		found := false

		for _, d := range all {
			if d.Name == dataset.Name {
				found = true

				break
			}
		}

		if found {
			continue
		}

		val := dataset.Properties[prop]
		if (dataset.Properties["mounted"] == "yes" || dataset.Properties["type"] == "volume") &&
			(val == "true" || val == "mysql" || val == "postgresql") {
			*included = append(*included, dataset)
		} else if val != "" {
			*excluded = append(*excluded, dataset)
		}
	}
}

func findRecursiveDatasets(datasets map[string][]zfs.Dataset) map[string][]zfs.Dataset {
	all := append([]zfs.Dataset{}, datasets["included"]...)
	all = append(all, datasets["excluded"]...)

	var single, recursive, cleanedRecursive []zfs.Dataset

	for _, dataset := range datasets["included"] {
		excludedChild := false

		for _, child := range all {
			if strings.HasPrefix(child.Name, dataset.Name) {
				for _, ex := range datasets["excluded"] {
					if ex.Name == child.Name {
						excludedChild = true

						single = append(single, dataset)

						break
					}
				}

				if excludedChild {
					break
				}
			}
		}

		if !excludedChild {
			recursive = append(recursive, dataset)
		}
	}

	for _, dataset := range recursive {
		var parent *zfs.Dataset

		if strings.Contains(dataset.Name, "/") {
			prefix := dataset.Name[:strings.LastIndex(dataset.Name, "/")]

			for _, d := range all {
				if d.Name == prefix {
					parent = &d

					break
				}
			}
		}

		if parent == nil || parent.Name == dataset.Name {
			cleanedRecursive = append(cleanedRecursive, dataset)
		} else {
			inRecursive := false

			for _, r := range recursive {
				if r.Name == parent.Name {
					inRecursive = true

					break
				}
			}

			if !inRecursive {
				cleanedRecursive = append(cleanedRecursive, dataset)
			}
		}
	}

	for i := range cleanedRecursive {
		parent := &cleanedRecursive[i]
		for _, d := range all {
			if strings.HasPrefix(d.Name, parent.Name+"/") && d.DB != "" {
				parent.DB = d.DB
			}
		}
	}

	return map[string][]zfs.Dataset{
		"single":    single,
		"recursive": cleanedRecursive,
		"included":  datasets["included"],
		"excluded":  datasets["excluded"],
	}
}

func FindEligibleDatasets(cfg config.Config, pool string) map[string][]zfs.Dataset {
	props := []string{
		snapshotProperty() + ":" + cfg.Interval,
		snapshotProperty(),
		"mounted",
	}

	all := zfs.ListDatasets(pool, props, cfg.Debug)

	var included []zfs.Dataset

	var excluded []zfs.Dataset

	filterDatasets(all, &included, &excluded, snapshotProperty()+":"+cfg.Interval)
	filterDatasets(all, &included, &excluded, snapshotProperty())

	return findRecursiveDatasets(map[string][]zfs.Dataset{
		"included": included,
		"excluded": excluded,
	})
}

func DoNewSnapshots(cfg config.Config, datasets map[string][]zfs.Dataset) {
	name := snapshotName(cfg)
	createManyFn(name, datasets["single"], false, cfg.DryRun, cfg.Verbose, cfg.Debug, cfg.UseThreads)
	createManyFn(name, datasets["recursive"], true, cfg.DryRun, cfg.Verbose, cfg.Debug, cfg.UseThreads)
}

func GroupSnapshotsIntoDatasets(snaps []zfs.Snapshot, datasets []zfs.Dataset) map[string][]zfs.Snapshot {
	result := map[string][]zfs.Snapshot{}

	for _, snap := range snaps {
		parts := strings.SplitN(snap.Name, "@", 2)

		if len(parts) != 2 {
			continue
		}

		for _, ds := range datasets {
			if ds.Name == parts[0] {
				result[ds.Name] = append(result[ds.Name], snap)

				break
			}
		}
	}

	return result
}

func destroyZeroSizedSnapshots(snaps []zfs.Snapshot, cfg config.Config) []zfs.Snapshot {
	if len(snaps) == 0 {
		return nil
	}

	// retain the newest snapshot (first in list)
	keep := []zfs.Snapshot{snaps[0]}

	for _, snap := range snaps[1:] {
		if snap.IsZero(cfg.Debug) {
			if cfg.Verbose {
				fmt.Println("Destroying zero-sized snapshot:", snap.Name)
			}

			if !cfg.DryRun {
				destroySnapshotFn(snap.Name, cfg.DryRun, cfg.Debug)
			}
		} else {
			keep = append(keep, snap)
		}
	}

	return keep
}

func DatasetsDestroyZeroSizedSnapshots(grouped map[string][]zfs.Snapshot, cfg config.Config) map[string][]zfs.Snapshot {
	var waitGroup sync.WaitGroup

	for name, snaps := range grouped {
		waitGroup.Add(1)

		go func() {
			grouped[name] = destroyZeroSizedSnapshots(snaps, cfg)

			waitGroup.Done()
		}()

		if !cfg.UseThreads {
			waitGroup.Wait()
		}
	}

	waitGroup.Wait()

	return grouped
}

func CleanupExpiredSnapshots(cfg config.Config, pool string, datasets map[string][]zfs.Dataset) {
	snaps, _ := zfs.ListSnapshotsFn(pool, true, cfg.Debug)

	var filtered []zfs.Snapshot

	prefix := snapshotPrefixInterval(cfg)

	for _, s := range snaps {
		if strings.Contains(s.Name, prefix) {
			filtered = append(filtered, s)
		}
	}

	grouped := GroupSnapshotsIntoDatasets(filtered, append(datasets["included"], datasets["excluded"]...))

	// keep only datasets we include
	for name := range grouped {
		found := false

		for _, ds := range datasets["included"] {
			if ds.Name == name {
				found = true

				break
			}
		}

		if !found {
			delete(grouped, name)
		}
	}

	if cfg.ShouldDestroyZeroSized {
		grouped = DatasetsDestroyZeroSizedSnapshots(grouped, cfg)
	}

	for name := range grouped {
		snaps := grouped[name]
		if len(snaps) > cfg.Keep {
			grouped[name] = snaps[cfg.Keep:]
		} else {
			grouped[name] = nil
		}
	}

	var waitGroup sync.WaitGroup

	for _, snaps := range grouped {
		for _, snap := range snaps {
			s := snap

			waitGroup.Add(1)

			go func() {
				destroySnapshotFn(s.Name, cfg.DryRun, cfg.Debug)
				waitGroup.Done()
			}()

			if !cfg.UseThreads {
				waitGroup.Wait()
			}
		}
	}

	waitGroup.Wait()
}
