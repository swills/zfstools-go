package zfstools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"zfstools-go/internal/config"
	"zfstools-go/internal/zfs"
)

type zeroSizePlan struct {
	retained map[string][]zfs.Snapshot
	destroy  []zfs.Snapshot
}

func planZeroSizeCleanup(
	ctx context.Context,
	grouped map[string][]zfs.Snapshot,
	debug bool,
) (zeroSizePlan, error) {
	plan := zeroSizePlan{retained: make(map[string][]zfs.Snapshot, len(grouped))}

	for name, snapshots := range grouped {
		if len(snapshots) == 0 {
			plan.retained[name] = nil

			continue
		}

		retained := []zfs.Snapshot{snapshots[0]}
		for index := 1; index < len(snapshots); index++ {
			snapshot := &snapshots[index]

			used, err := snapshot.GetUsed(ctx, debug)
			if err != nil {
				return zeroSizePlan{}, fmt.Errorf("get used size for %s: %w", snapshot.Name, err)
			}

			if used == 0 {
				plan.destroy = append(plan.destroy, *snapshot)
			} else {
				retained = append(retained, *snapshot)
			}
		}

		plan.retained[name] = retained
	}

	return plan, nil
}

func (tools Tools) applyZeroSizePlan(ctx context.Context, plan zeroSizePlan, cfg config.Config) error {
	if cfg.Verbose {
		for _, snapshot := range plan.destroy {
			_, _ = fmt.Fprintln(tools.output, "Destroying zero-sized snapshot:", snapshot.Name)
		}
	}

	if err := tools.destroySnapshots(ctx, plan.destroy, cfg); err != nil {
		return fmt.Errorf("destroy zero-sized snapshots: %w", err)
	}

	return nil
}

func (tools Tools) destroySnapshots(ctx context.Context, snapshots []zfs.Snapshot, cfg config.Config) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("destroy snapshots: %w", err)
	}

	if !cfg.UseThreads {
		for _, snapshot := range snapshots {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("destroy snapshots: %w", err)
			}

			if err := tools.client.DestroySnapshot(ctx, snapshot.Name, cfg.DryRun, cfg.Debug); err != nil {
				return fmt.Errorf("destroy snapshot %s: %w", snapshot.Name, err)
			}
		}

		return nil
	}

	results := make(chan error, len(snapshots))

	for _, snapshot := range snapshots {
		go func() {
			err := tools.client.DestroySnapshot(ctx, snapshot.Name, cfg.DryRun, cfg.Debug)
			if err != nil {
				err = fmt.Errorf("destroy snapshot %s: %w", snapshot.Name, err)
			}

			results <- err
		}()
	}

	var result error

	for range snapshots {
		result = errors.Join(result, <-results)
	}

	return result
}

// PruneZeroSizedSnapshots removes manual zero-sized snapshots while retaining
// the newest snapshot for each dataset.
func (tools Tools) PruneZeroSizedSnapshots(ctx context.Context, cfg config.Config, pool string) error {
	snapshots, err := tools.client.ListSnapshots(ctx, pool, true, cfg.Debug)
	if err != nil {
		return fmt.Errorf("list snapshots for zero-size cleanup: %w", err)
	}

	filtered := make([]zfs.Snapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if !strings.Contains(snapshot.Name, "zfs-auto-snap_") {
			filtered = append(filtered, snapshot)
		}
	}

	datasets, err := tools.client.ListDatasets(ctx, pool, nil, cfg.Debug)
	if err != nil {
		return fmt.Errorf("list datasets for zero-size cleanup: %w", err)
	}

	grouped := GroupSnapshotsIntoDatasets(filtered, datasets)

	plan, err := planZeroSizeCleanup(ctx, grouped, cfg.Debug)
	if err != nil {
		return fmt.Errorf("plan zero-size cleanup: %w", err)
	}

	return tools.applyZeroSizePlan(ctx, plan, cfg)
}

// ApplySnapshotRetention removes generated snapshots excluded by the current
// zero-size and retention-count policies.
func (tools Tools) ApplySnapshotRetention(
	ctx context.Context,
	cfg config.Config,
	pool string,
	datasets map[string][]zfs.Dataset,
	retentionTargets map[string]struct{},
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("apply snapshot retention: %w", err)
	}

	if len(retentionTargets) == 0 {
		return nil
	}

	snapshots, err := tools.client.ListSnapshots(ctx, pool, true, cfg.Debug)
	if err != nil {
		return fmt.Errorf("list snapshots for retention: %w", err)
	}

	var filtered []zfs.Snapshot

	prefix := snapshotPrefixInterval(cfg)

	for _, snapshot := range snapshots {
		if strings.Contains(snapshot.Name, prefix) {
			filtered = append(filtered, snapshot)
		}
	}

	grouped := GroupSnapshotsIntoDatasets(filtered, append(datasets["included"], datasets["excluded"]...))

	for name := range grouped {
		if _, ok := retentionTargets[name]; !ok {
			delete(grouped, name)
		}
	}

	if cfg.ShouldDestroyZeroSized {
		plan, planErr := planZeroSizeCleanup(ctx, grouped, cfg.Debug)
		if planErr != nil {
			return fmt.Errorf("plan zero-size retention: %w", planErr)
		}

		if err := tools.applyZeroSizePlan(ctx, plan, cfg); err != nil {
			return err
		}

		grouped = plan.retained
	}

	var expired []zfs.Snapshot

	for _, groupedSnapshots := range grouped {
		if len(groupedSnapshots) > cfg.Keep {
			expired = append(expired, groupedSnapshots[cfg.Keep:]...)
		}
	}

	if err := tools.destroySnapshots(ctx, expired, cfg); err != nil {
		return fmt.Errorf("apply snapshot retention: %w", err)
	}

	return nil
}
