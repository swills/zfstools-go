package zfstools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"zfstools-go/internal/config"
	"zfstools-go/internal/zfs"
)

const snapshotProperty = "com.sun:auto-snapshot"

const snapshotFormat = "2006-01-02-15h04"

type zfsClient interface {
	ListDatasets(ctx context.Context, pool string, properties []string, debug bool) ([]zfs.Dataset, error)
	ListSnapshots(ctx context.Context, dataset string, recursive, debug bool) ([]zfs.Snapshot, error)
	CreateManySnapshots(
		ctx context.Context,
		snapshotName string,
		datasets []zfs.Dataset,
		recursive bool,
		dryRun, verbose, debug, useThreads bool,
	) error
	DestroySnapshot(ctx context.Context, name string, dryRun, debug bool) error
}

type Tools struct {
	client zfsClient
	output io.Writer
}

// New creates orchestration tools backed by a ZFS client.
func New(client zfsClient, output io.Writer) Tools {
	return Tools{client: client, output: output}
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

func snapshotName(cfg config.Config) string {
	timestamp := cfg.Timestamp
	if cfg.UseUTC {
		timestamp = timestamp.UTC()

		return snapshotPrefixInterval(cfg) + timestamp.Format(snapshotFormat) + "U"
	}

	return snapshotPrefixInterval(cfg) + timestamp.Format(snapshotFormat)
}

// filterDatasets does the filtering work for FindEligibleDatasets
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

// findRecursiveDatasets helps FindEligibleDatasets decide which datasets can be snapshot recursively
func findRecursiveDatasets(datasets map[string][]zfs.Dataset) map[string][]zfs.Dataset {
	all := append([]zfs.Dataset{}, datasets["included"]...)
	all = append(all, datasets["excluded"]...)
	excludedNames := make(map[string]struct{}, len(datasets["excluded"]))

	for _, dataset := range datasets["excluded"] {
		excludedNames[dataset.Name] = struct{}{}
	}

	single, recursive := partitionRecursiveDatasets(datasets["included"], all, excludedNames)
	cleanedRecursive := removeRecursiveChildren(all, recursive)

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

func partitionRecursiveDatasets(
	included, all []zfs.Dataset,
	excludedNames map[string]struct{},
) ([]zfs.Dataset, []zfs.Dataset) {
	var single, recursive []zfs.Dataset

	for _, dataset := range included {
		if hasExcludedChild(dataset, all, excludedNames) {
			single = append(single, dataset)
		} else {
			recursive = append(recursive, dataset)
		}
	}

	return single, recursive
}

func hasExcludedChild(dataset zfs.Dataset, all []zfs.Dataset, excludedNames map[string]struct{}) bool {
	for _, child := range all {
		if !strings.HasPrefix(child.Name, dataset.Name+"/") {
			continue
		}

		if _, excluded := excludedNames[child.Name]; excluded {
			return true
		}
	}

	return false
}

func removeRecursiveChildren(all, recursive []zfs.Dataset) []zfs.Dataset {
	allNames := make(map[string]struct{}, len(all))
	recursiveNames := make(map[string]struct{}, len(recursive))

	for _, dataset := range all {
		allNames[dataset.Name] = struct{}{}
	}

	for _, dataset := range recursive {
		recursiveNames[dataset.Name] = struct{}{}
	}

	var cleaned []zfs.Dataset

	for _, dataset := range recursive {
		separator := strings.LastIndex(dataset.Name, "/")
		if separator == -1 {
			cleaned = append(cleaned, dataset)

			continue
		}

		parentName := dataset.Name[:separator]
		_, parentExists := allNames[parentName]
		_, parentIsRecursive := recursiveNames[parentName]

		if !parentExists || !parentIsRecursive {
			cleaned = append(cleaned, dataset)
		}
	}

	return cleaned
}

// FindEligibleDatasets returns datasets eligible for snapshotting, grouped into four categories:
// - single: datasets which cannot be snapshot recursively and must be done individually
// - recursive: datasets which can be snapshot recursively, since all snapshots below them are eligible as well
// - included: datasets which were included in one of those two lists
// - excluded: datasets which were excluded from both of those lists
func (tools Tools) FindEligibleDatasets(
	ctx context.Context,
	cfg config.Config,
	pool string,
) (map[string][]zfs.Dataset, error) {
	props := []string{
		snapshotProperty + ":" + cfg.Interval,
		snapshotProperty,
		"mounted",
	}

	all, err := tools.client.ListDatasets(ctx, pool, props, cfg.Debug)
	if err != nil {
		return nil, fmt.Errorf("find eligible datasets: %w", err)
	}

	var included []zfs.Dataset

	var excluded []zfs.Dataset

	filterDatasets(all, &included, &excluded, snapshotProperty+":"+cfg.Interval)
	filterDatasets(all, &included, &excluded, snapshotProperty)

	return findRecursiveDatasets(map[string][]zfs.Dataset{
		"included": included,
		"excluded": excluded,
	}), nil
}

// DoNewSnapshots creates the single and recursive snapshots.
func (tools Tools) DoNewSnapshots(
	ctx context.Context,
	cfg config.Config,
	datasets map[string][]zfs.Dataset,
) error {
	name := snapshotName(cfg)

	var singleErr, recursiveErr error

	if len(datasets["single"]) > 0 {
		singleErr = tools.client.CreateManySnapshots(
			ctx,
			name, datasets["single"], false, cfg.DryRun, cfg.Verbose, cfg.Debug, cfg.UseThreads,
		)
	}

	if len(datasets["recursive"]) > 0 {
		recursiveErr = tools.client.CreateManySnapshots(
			ctx,
			name, datasets["recursive"], true, cfg.DryRun, cfg.Verbose, cfg.Debug, cfg.UseThreads,
		)
	}

	return errors.Join(singleErr, recursiveErr)
}

func GroupSnapshotsIntoDatasets(snaps []zfs.Snapshot, datasets []zfs.Dataset) map[string][]zfs.Snapshot {
	result := map[string][]zfs.Snapshot{}

	for _, snap := range snaps {
		datasetName, _, found := strings.Cut(snap.Name, "@")
		if !found {
			continue
		}

		for _, ds := range datasets {
			if ds.Name == datasetName {
				result[ds.Name] = append(result[ds.Name], snap)

				break
			}
		}
	}

	return result
}
