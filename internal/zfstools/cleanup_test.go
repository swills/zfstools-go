package zfstools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/go-test/deep"

	"zfstools-go/internal/config"
	"zfstools-go/internal/zfs"
)

func retentionTargets(names ...string) map[string]struct{} {
	targets := make(map[string]struct{}, len(names))
	for _, name := range names {
		targets[name] = struct{}{}
	}

	return targets
}

func TestApplySnapshotRetentionCancellationSkipsCleanup(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	runner := &fakeRunner{}
	client := zfs.NewClient(runner, io.Discard)

	err := New(client, io.Discard).ApplySnapshotRetention(
		ctx,
		config.Config{Keep: 1},
		"tank",
		map[string][]zfs.Dataset{"included": {{Name: "tank/a"}}},
		retentionTargets("tank/a"),
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ApplySnapshotRetention() error = %v, want context cancellation", err)
	}

	if len(runner.calls) != 0 {
		t.Errorf("Run calls = %v, want none", runner.calls)
	}
}

func TestDestroySnapshotsCancellation(t *testing.T) {
	t.Parallel()

	t.Run("before cleanup", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		runner := &fakeRunner{}
		client := zfs.NewClient(runner, io.Discard)

		err := New(client, io.Discard).destroySnapshots(
			ctx, []zfs.Snapshot{{Name: "tank/a@1"}}, config.Config{},
		)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("destroySnapshots() error = %v, want context cancellation", err)
		}

		if len(runner.calls) != 0 {
			t.Errorf("Run calls = %v, want none", runner.calls)
		}
	})

	t.Run("during serial cleanup", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		runner := &fakeRunner{runFunc: func(_ string, args ...string) ([]byte, error) {
			if args[0] == "destroy" {
				cancel()
			}

			return nil, nil
		}}
		client := zfs.NewClient(runner, io.Discard)

		err := New(client, io.Discard).destroySnapshots(
			ctx,
			[]zfs.Snapshot{{Name: "tank/a@2"}, {Name: "tank/a@1"}},
			config.Config{},
		)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("destroySnapshots() error = %v, want context cancellation", err)
		}

		if len(runner.calls) != 1 || runner.calls[0].args[len(runner.calls[0].args)-1] != "tank/a@2" {
			t.Errorf("Run calls = %v, want only first destruction", runner.calls)
		}
	})
}

func TestDestroyZeroSizedSnapshots(t *testing.T) {
	t.Parallel()

	type args struct {
		snaps []zfs.Snapshot
		cfg   config.Config
	}

	tests := []struct {
		name string
		want []zfs.Snapshot
		args args
	}{
		{
			name: "zeroSnapshots",
			args: args{
				snaps: nil,
				cfg:   config.Config{},
			},
		},
		{
			name: "oneSnapshotNotZero",
			args: args{
				snaps: []zfs.Snapshot{
					{
						Name: "tank/a@1",
						Used: 123456,
					},
				},
				cfg: config.Config{},
			},
			want: []zfs.Snapshot{
				{
					Name: "tank/a@1",
					Used: 123456,
				},
			},
		},
		{
			name: "oneSnapshotZero",
			args: args{
				snaps: []zfs.Snapshot{
					{
						Name: "tank/a@1",
						Used: 0,
					},
				},
				cfg: config.Config{},
			},
			want: []zfs.Snapshot{
				{
					Name: "tank/a@1",
					Used: 0,
				},
			},
		},
		{
			name: "twoSnapshotsNeitherZero",
			args: args{
				snaps: []zfs.Snapshot{
					{
						Name: "tank/a@2",
						Used: 123456,
					},
					{
						Name: "tank/a@1",
						Used: 12345,
					},
				},
				cfg: config.Config{},
			},
			want: []zfs.Snapshot{
				{
					Name: "tank/a@2",
					Used: 123456,
				},
				{
					Name: "tank/a@1",
					Used: 12345,
				},
			},
		},
		{
			name: "twoSnapshotsFirstZero",
			args: args{
				snaps: []zfs.Snapshot{
					{
						Name: "tank/a@2",
						Used: 123456,
					},
					{
						Name: "tank/a@1",
						Used: 0,
					},
				},
				cfg: config.Config{},
			},
			want: []zfs.Snapshot{
				{
					Name: "tank/a@2",
					Used: 123456,
				},
			},
		},
		{
			name: "twoSnapshotsSecondZero",
			args: args{
				snaps: []zfs.Snapshot{
					{
						Name: "tank/a@2",
						Used: 0,
					},
					{
						Name: "tank/a@1",
						Used: 123456,
					},
				},
				cfg: config.Config{},
			},
			want: []zfs.Snapshot{
				{
					Name: "tank/a@2",
					Used: 0,
				},
				{
					Name: "tank/a@1",
					Used: 123456,
				},
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var listing bytes.Buffer
			for _, snapshot := range testCase.args.snaps {
				_, _ = fmt.Fprintf(&listing, "%s\t%d\n", snapshot.Name, snapshot.Used)
			}

			runner := &fakeRunner{output: listing.Bytes()}
			client := zfs.NewClient(runner, io.Discard)

			snapshots, err := client.ListSnapshots(t.Context(), "", true, false)
			if err != nil {
				t.Fatalf("ListSnapshots() error = %v", err)
			}

			plan, err := planZeroSizeCleanup(
				t.Context(), map[string][]zfs.Snapshot{"tank/a": snapshots}, testCase.args.cfg.Debug,
			)
			if err != nil {
				t.Fatalf("planZeroSizeCleanup() error = %v", err)
			}

			if err := New(client, io.Discard).applyZeroSizePlan(t.Context(), plan, testCase.args.cfg); err != nil {
				t.Fatalf("applyZeroSizePlan() error = %v", err)
			}

			got := plan.retained["tank/a"]

			for i := range got {
				got[i] = zfs.Snapshot{Name: got[i].Name, Used: got[i].Used}
			}

			diff := deep.Equal(got, testCase.want)
			if diff != nil {
				t.Errorf("compare failed: %#v", diff)
			}
		})
	}
}

func TestDestroyZeroSizedSnapshotsRetainsUnknownSize(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{runFunc: func(_ string, args ...string) ([]byte, error) {
		switch args[0] {
		case "list":
			return []byte("tank/a@3\t1\ntank/a@2\t0\ntank/a@1\t0\n"), nil
		case "get":
			return nil, errTestCommand
		default:
			return nil, nil
		}
	}}

	client := zfs.NewClient(runner, io.Discard)

	snapshots, err := client.ListSnapshots(t.Context(), "", true, false)
	if err != nil {
		t.Fatalf("ListSnapshots() error = %v", err)
	}

	if setupErr := client.DestroySnapshot(t.Context(), "tank/a@unrelated", false, false); setupErr != nil {
		t.Fatalf("DestroySnapshot() setup error = %v", setupErr)
	}

	runner.mu.Lock()
	runner.calls = nil
	runner.mu.Unlock()

	_, err = planZeroSizeCleanup(t.Context(), map[string][]zfs.Snapshot{"tank/a": snapshots}, false)
	if !errors.Is(err, errTestCommand) {
		t.Fatalf("planZeroSizeCleanup() error = %v, want size error", err)
	}

	var destroyed []string

	for _, call := range runner.calls {
		if call.name == "zfs" && len(call.args) > 0 && call.args[0] == "destroy" {
			destroyed = append(destroyed, call.args[len(call.args)-1])
		}
	}

	if len(destroyed) != 0 {
		t.Errorf("destroyed snapshots = %v, want none", destroyed)
	}
}

func TestDestroyZeroSizedSnapshotsReportsDestroyError(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{runFunc: func(_ string, args ...string) ([]byte, error) {
		if args[0] == "list" {
			return []byte("tank/a@2\t1\ntank/a@1\t0\n"), nil
		}

		if args[0] == "destroy" {
			return nil, errTestCommand
		}

		return nil, nil
	}}
	client := zfs.NewClient(runner, io.Discard)

	snapshots, err := client.ListSnapshots(t.Context(), "", true, false)
	if err != nil {
		t.Fatalf("ListSnapshots() error = %v", err)
	}

	plan, err := planZeroSizeCleanup(t.Context(), map[string][]zfs.Snapshot{"tank/a": snapshots}, false)
	if err != nil {
		t.Fatalf("planZeroSizeCleanup() error = %v", err)
	}

	err = New(client, io.Discard).applyZeroSizePlan(t.Context(), plan, config.Config{})
	if !errors.Is(err, errTestCommand) {
		t.Fatalf("applyZeroSizePlan() error = %v, want destroy error", err)
	}
}

func TestDestroyZeroSizedSnapshotsVerboseOutput(t *testing.T) {
	t.Parallel()

	output := &bytes.Buffer{}
	runner := &fakeRunner{output: []byte("tank/a@2\t0\ntank/a@1\t0\n")}
	client := zfs.NewClient(runner, output)

	snapshots, err := client.ListSnapshots(t.Context(), "", true, false)
	if err != nil {
		t.Fatalf("ListSnapshots() error = %v", err)
	}

	plan, err := planZeroSizeCleanup(t.Context(), map[string][]zfs.Snapshot{"tank/a": snapshots}, false)
	if err != nil {
		t.Fatalf("planZeroSizeCleanup() error = %v", err)
	}

	err = New(client, output).applyZeroSizePlan(t.Context(), plan, config.Config{DryRun: true, Verbose: true})
	if err != nil {
		t.Fatalf("applyZeroSizePlan() error = %v", err)
	}

	want := "Destroying zero-sized snapshot: tank/a@1\n"
	if got := output.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestApplyZeroSizePlanParallel(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		runFunc: func(name string, args ...string) ([]byte, error) {
			if name == "zfs" && len(args) > 0 {
				switch args[0] {
				case "list":
					return []byte("tank/a@2\t1\ntank/a@1\t0\ntank/b@2\t1\ntank/b@1\t0\n"), nil
				case "get":
					return []byte("0\n"), nil
				}
			}

			return nil, nil
		},
	}
	client := zfs.NewClient(runner, io.Discard)
	tools := New(client, io.Discard)

	snapshots, err := client.ListSnapshots(t.Context(), "", true, false)
	if err != nil {
		t.Fatalf("ListSnapshots() error = %v", err)
	}

	grouped := GroupSnapshotsIntoDatasets(snapshots, []zfs.Dataset{{Name: "tank/a"}, {Name: "tank/b"}})

	plan, err := planZeroSizeCleanup(t.Context(), grouped, false)
	if err != nil {
		t.Fatalf("planZeroSizeCleanup() error = %v", err)
	}

	if err := tools.applyZeroSizePlan(t.Context(), plan, config.Config{UseThreads: true}); err != nil {
		t.Fatalf("applyZeroSizePlan() error = %v", err)
	}

	for name, snapshots := range plan.retained {
		if len(snapshots) != 1 || snapshots[0].Name != name+"@2" {
			t.Errorf("snapshots for %s = %#v, want newest snapshot only", name, snapshots)
		}
	}

	var destroyed []string

	for _, call := range runner.calls {
		if call.name == "zfs" && len(call.args) > 0 && call.args[0] == "destroy" {
			destroyed = append(destroyed, call.args[len(call.args)-1])
		}
	}

	slices.Sort(destroyed)

	if diff := deep.Equal(destroyed, []string{"tank/a@1", "tank/b@1"}); diff != nil {
		t.Errorf("destroyed snapshots differ: %v", diff)
	}
}

func TestApplyZeroSizePlanParallelErrors(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{runFunc: func(_ string, args ...string) ([]byte, error) {
		if args[0] == "list" {
			return []byte("tank/a@2\t1\ntank/a@1\t0\ntank/b@2\t1\ntank/b@1\t0\n"), nil
		}

		if args[0] != "destroy" {
			return nil, nil
		}

		if strings.HasPrefix(args[len(args)-1], "tank/a@") {
			return nil, errTestCommand
		}

		return nil, errSecondTestCommand
	}}
	client := zfs.NewClient(runner, io.Discard)

	snapshots, err := client.ListSnapshots(t.Context(), "", true, false)
	if err != nil {
		t.Fatalf("ListSnapshots() error = %v", err)
	}

	grouped := GroupSnapshotsIntoDatasets(snapshots, []zfs.Dataset{{Name: "tank/a"}, {Name: "tank/b"}})

	plan, err := planZeroSizeCleanup(t.Context(), grouped, false)
	if err != nil {
		t.Fatalf("planZeroSizeCleanup() error = %v", err)
	}

	err = New(client, io.Discard).applyZeroSizePlan(t.Context(), plan, config.Config{UseThreads: true})
	if !errors.Is(err, errTestCommand) || !errors.Is(err, errSecondTestCommand) {
		t.Fatalf("applyZeroSizePlan() error = %v, want both destroy errors", err)
	}
}

func TestPlanZeroSizeCleanupCompletesBeforeDestroying(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{runFunc: func(_ string, args ...string) ([]byte, error) {
		switch args[0] {
		case "list":
			return []byte("tank/a@2\t1\ntank/a@1\t0\ntank/b@2\t1\ntank/b@1\t0\n"), nil
		case "get":
			if strings.HasPrefix(args[len(args)-1], "tank/b@") {
				return nil, errTestCommand
			}

			return []byte("0\n"), nil
		default:
			return nil, nil
		}
	}}
	client := zfs.NewClient(runner, io.Discard)

	snapshots, err := client.ListSnapshots(t.Context(), "", true, false)
	if err != nil {
		t.Fatalf("ListSnapshots() error = %v", err)
	}

	if setupErr := client.DestroySnapshot(t.Context(), "tank/a@unrelated", false, false); setupErr != nil {
		t.Fatalf("DestroySnapshot() setup error = %v", setupErr)
	}

	runner.mu.Lock()
	runner.calls = nil
	runner.mu.Unlock()

	grouped := GroupSnapshotsIntoDatasets(snapshots, []zfs.Dataset{{Name: "tank/a"}, {Name: "tank/b"}})

	_, err = planZeroSizeCleanup(t.Context(), grouped, false)
	if !errors.Is(err, errTestCommand) {
		t.Fatalf("planZeroSizeCleanup() error = %v, want size error", err)
	}

	for _, call := range runner.calls {
		if call.name == "zfs" && len(call.args) > 0 && call.args[0] == "destroy" {
			t.Errorf("unexpected destroy command before planning completed: %v", call.args)
		}
	}
}

func TestApplySnapshotRetention(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{output: []byte(
		"tank/included@zfs-auto-snap_daily-new\t10\n" +
			"tank/included@zfs-auto-snap_daily-old\t10\n" +
			"tank/within-keep@zfs-auto-snap_daily-only\t10\n" +
			"tank/excluded@zfs-auto-snap_daily-old\t10\n" +
			"tank/included@manual\t10\n",
	)}
	client := zfs.NewClient(runner, io.Discard)
	tools := New(client, io.Discard)
	datasets := map[string][]zfs.Dataset{
		"included": {{Name: "tank/included"}, {Name: "tank/within-keep"}},
		"excluded": {{Name: "tank/excluded"}},
	}
	cfg := config.Config{Interval: "daily", Keep: 1, ShouldDestroyZeroSized: true}

	if err := tools.ApplySnapshotRetention(
		t.Context(), cfg, "tank", datasets, retentionTargets("tank/included", "tank/within-keep"),
	); err != nil {
		t.Fatalf("ApplySnapshotRetention() error = %v", err)
	}

	var destroyed []string

	for _, call := range runner.calls {
		if call.name == "zfs" && len(call.args) > 0 && call.args[0] == "destroy" {
			destroyed = append(destroyed, call.args[len(call.args)-1])
		}
	}

	if diff := deep.Equal(destroyed, []string{"tank/included@zfs-auto-snap_daily-old"}); diff != nil {
		t.Errorf("destroyed snapshots differ: %v", diff)
	}

	wantList := commandCall{name: "zfs", args: []string{
		"list", "-r", "-H", "-p", "-t", "snapshot", "-o", "name,used", "-S", "name", "tank",
	}}
	if diff := deep.Equal(runner.calls[0], wantList); diff != nil {
		t.Errorf("list command differs: %v", diff)
	}
}

func TestApplySnapshotRetentionWithoutTargetsDoesNothing(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{runFunc: func(string, ...string) ([]byte, error) {
		return nil, errTestCommand
	}}
	tools := New(zfs.NewClient(runner, io.Discard), io.Discard)

	if err := tools.ApplySnapshotRetention(t.Context(), config.Config{}, "tank", nil, nil); err != nil {
		t.Fatalf("ApplySnapshotRetention() error = %v", err)
	}

	if len(runner.calls) != 0 {
		t.Errorf("command count = %d, want none", len(runner.calls))
	}
}

func TestApplySnapshotRetentionListError(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{runFunc: func(string, ...string) ([]byte, error) {
		return nil, errTestCommand
	}}
	tools := New(zfs.NewClient(runner, io.Discard), io.Discard)

	err := tools.ApplySnapshotRetention(
		t.Context(), config.Config{}, "tank", nil, retentionTargets("tank/data"),
	)
	if !errors.Is(err, errTestCommand) {
		t.Fatalf("ApplySnapshotRetention() error = %v, want command error", err)
	}

	if len(runner.calls) != 1 {
		t.Errorf("command count = %d, want snapshot listing only", len(runner.calls))
	}
}

func TestApplySnapshotRetentionDestroyErrorStopsSerialCleanup(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{runFunc: func(_ string, args ...string) ([]byte, error) {
		if args[0] == "list" {
			return []byte(
				"tank/data@zfs-auto-snap_daily-new\t10\n" +
					"tank/data@zfs-auto-snap_daily-old\t10\n",
			), nil
		}

		if args[0] == "destroy" {
			return nil, errTestCommand
		}

		return nil, nil
	}}
	tools := New(zfs.NewClient(runner, io.Discard), io.Discard)
	datasets := map[string][]zfs.Dataset{"included": {{Name: "tank/data"}}}

	err := tools.ApplySnapshotRetention(
		t.Context(), config.Config{Interval: "daily"}, "tank", datasets, retentionTargets("tank/data"),
	)
	if !errors.Is(err, errTestCommand) {
		t.Fatalf("ApplySnapshotRetention() error = %v, want destroy error", err)
	}

	destroyCalls := 0

	for _, call := range runner.calls {
		if call.name == "zfs" && len(call.args) > 0 && call.args[0] == "destroy" {
			destroyCalls++
		}
	}

	if destroyCalls != 1 {
		t.Errorf("destroy command count = %d, want 1", destroyCalls)
	}
}

func TestApplySnapshotRetentionReportsZeroSizePlanError(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{runFunc: func(_ string, args ...string) ([]byte, error) {
		switch args[0] {
		case "list":
			return []byte(
				"tank/data@zfs-auto-snap_daily-new\t1\n" +
					"tank/data@zfs-auto-snap_daily-old\t0\n",
			), nil
		case "get":
			return nil, errTestCommand
		default:
			return nil, nil
		}
	}}

	client := zfs.NewClient(runner, io.Discard)
	if setupErr := client.DestroySnapshot(t.Context(), "tank/data@unrelated", false, false); setupErr != nil {
		t.Fatalf("DestroySnapshot() setup error = %v", setupErr)
	}

	runner.mu.Lock()
	runner.calls = nil
	runner.mu.Unlock()

	datasets := map[string][]zfs.Dataset{"included": {{Name: "tank/data"}}}

	err := New(client, io.Discard).ApplySnapshotRetention(
		t.Context(), config.Config{Interval: "daily", Keep: 1, ShouldDestroyZeroSized: true},
		"tank", datasets, retentionTargets("tank/data"),
	)
	if !errors.Is(err, errTestCommand) {
		t.Fatalf("ApplySnapshotRetention() error = %v, want size error", err)
	}

	for _, call := range runner.calls {
		if call.name == "zfs" && len(call.args) > 0 && call.args[0] == "destroy" {
			t.Errorf("unexpected destroy command: %v", call.args)
		}
	}
}

func TestApplySnapshotRetentionReportsZeroSizeDestroyError(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{runFunc: func(_ string, args ...string) ([]byte, error) {
		if args[0] == "list" {
			return []byte(
				"tank/data@zfs-auto-snap_daily-new\t1\n" +
					"tank/data@zfs-auto-snap_daily-old\t0\n",
			), nil
		}

		if args[0] == "destroy" {
			return nil, errTestCommand
		}

		return nil, nil
	}}
	datasets := map[string][]zfs.Dataset{"included": {{Name: "tank/data"}}}
	tools := New(zfs.NewClient(runner, io.Discard), io.Discard)

	err := tools.ApplySnapshotRetention(
		t.Context(), config.Config{Interval: "daily", Keep: 1, ShouldDestroyZeroSized: true},
		"tank", datasets, retentionTargets("tank/data"),
	)
	if !errors.Is(err, errTestCommand) {
		t.Fatalf("ApplySnapshotRetention() error = %v, want destroy error", err)
	}
}

func TestPruneZeroSizedSnapshots(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{runFunc: func(_ string, args ...string) ([]byte, error) {
		if args[0] != "list" {
			return nil, nil
		}

		if slices.Contains(args, "snapshot") {
			return []byte("tank/data@manual-new\t0\ntank/data@manual-old\t0\n"), nil
		}

		return []byte("tank/data\tfilesystem\n"), nil
	}}
	tools := New(zfs.NewClient(runner, io.Discard), io.Discard)

	if err := tools.PruneZeroSizedSnapshots(t.Context(), config.Config{}, "tank"); err != nil {
		t.Fatalf("PruneZeroSizedSnapshots() error = %v", err)
	}

	var destroyed []string

	for _, call := range runner.calls {
		if call.name == "zfs" && len(call.args) > 0 && call.args[0] == "destroy" {
			destroyed = append(destroyed, call.args[len(call.args)-1])
		}
	}

	if !slices.Equal(destroyed, []string{"tank/data@manual-old"}) {
		t.Errorf("destroyed snapshots = %v, want manual-old", destroyed)
	}
}

func TestPruneZeroSizedSnapshotsDiscoveryErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		failDatasets bool
	}{
		{name: "snapshots"},
		{name: "datasets", failDatasets: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			runner := &fakeRunner{runFunc: func(_ string, args ...string) ([]byte, error) {
				isSnapshotList := args[0] == "list" && slices.Contains(args, "snapshot")
				if isSnapshotList == testCase.failDatasets {
					if isSnapshotList {
						return []byte("tank/data@manual\t0\n"), nil
					}

					return []byte("tank/data\tfilesystem\n"), nil
				}

				return nil, errTestCommand
			}}
			tools := New(zfs.NewClient(runner, io.Discard), io.Discard)

			err := tools.PruneZeroSizedSnapshots(t.Context(), config.Config{}, "tank")
			if !errors.Is(err, errTestCommand) {
				t.Fatalf("PruneZeroSizedSnapshots() error = %v, want discovery error", err)
			}
		})
	}
}

func TestPruneZeroSizedSnapshotsReportsPlanError(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{runFunc: func(_ string, args ...string) ([]byte, error) {
		switch {
		case args[0] == "list" && slices.Contains(args, "snapshot"):
			return []byte("tank/data@manual-new\t1\ntank/data@manual-old\t0\n"), nil
		case args[0] == "list":
			return []byte("tank/data\tfilesystem\n"), nil
		case args[0] == "get":
			return nil, errTestCommand
		default:
			return nil, nil
		}
	}}

	client := zfs.NewClient(runner, io.Discard)
	if setupErr := client.DestroySnapshot(t.Context(), "tank/data@unrelated", false, false); setupErr != nil {
		t.Fatalf("DestroySnapshot() setup error = %v", setupErr)
	}

	runner.mu.Lock()
	runner.calls = nil
	runner.mu.Unlock()

	err := New(client, io.Discard).PruneZeroSizedSnapshots(t.Context(), config.Config{}, "tank")
	if !errors.Is(err, errTestCommand) {
		t.Fatalf("PruneZeroSizedSnapshots() error = %v, want size error", err)
	}

	for _, call := range runner.calls {
		if call.name == "zfs" && len(call.args) > 0 && call.args[0] == "destroy" {
			t.Errorf("unexpected destroy command: %v", call.args)
		}
	}
}
