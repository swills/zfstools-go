package zfs

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	argMaxFallback     = 4096
	argMaxSafetyMargin = 1024
	snapshotFieldCount = 2
)

var ErrEmptySnapshotName = errors.New("empty snapshot name")

var ErrInvalidSnapshotName = errors.New("invalid snapshot name")

var ErrNoDatasets = errors.New("no dataset(s) specified")

var ErrOneSnapshotOfManyErrored = errors.New("some snapshots failed to create")

var errInvalidSnapshotOutput = errors.New("invalid zfs snapshot output")

type Snapshot struct {
	runner    Runner
	output    io.Writer
	state     *snapshotState
	Name      string
	Used      int64
	usedKnown bool
}

// GetUsed returns the used size of the snapshot, refreshing it when the value
// is unknown or stale. It returns refresh and parsing failures separately from
// a confirmed size of zero.
func (s *Snapshot) GetUsed(ctx context.Context, debug bool) (int64, error) {
	runner, output, state := s.runner, s.output, s.state
	if runner == nil {
		client := NewSystemClient(io.Discard)
		runner, output, state = client.runner, client.output, client.snapshotState
	}

	if (!s.usedKnown && s.Used == 0) || state.stale.Load() {
		if debug {
			_, _ = fmt.Fprintln(output, "zfs get -Hp -o value used", s.Name)
		}

		var out bytes.Buffer

		err := runner.Run(ctx, &out, "zfs", "get", "-Hp", "-o", "value", "used", s.Name)
		if err != nil {
			return 0, fmt.Errorf("get used size for %s: %w", s.Name, err)
		}

		used, err := strconv.ParseInt(strings.TrimSpace(out.String()), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse used size for %s: %w", s.Name, err)
		}

		s.Used = used
		s.usedKnown = true
	}

	return s.Used, nil
}

// ListSnapshots returns snapshots using the client's command runner.
func (client Client) ListSnapshots(
	ctx context.Context,
	dataset string,
	recursive bool,
	debug bool,
) ([]Snapshot, error) {
	args := []string{"list"}

	if dataset != "" && !recursive {
		args = append(args, "-d", "1")
	}

	if recursive {
		args = append(args, "-r")
	}

	args = append(args, "-H", "-p", "-t", "snapshot", "-o", "name,used", "-S", "name")

	if dataset != "" {
		args = append(args, dataset)
	}

	if debug {
		_, _ = fmt.Fprintln(client.output, "zfs", strings.Join(args, " "))
	}

	reader, done := client.stream(ctx, "zfs", args...)
	snapshots, parseErr := parseSnapshots(reader, client.runner, client.output, client.snapshotState)
	_ = reader.Close()

	runErr := <-done
	if runErr != nil {
		runErr = fmt.Errorf("error listing snapshots: %w", runErr)
	}

	if err := errors.Join(parseErr, runErr); err != nil {
		return nil, err
	}

	return snapshots, nil
}

func parseSnapshots(
	reader io.Reader,
	runner Runner,
	output io.Writer,
	state *snapshotState,
) ([]Snapshot, error) {
	var snapshots []Snapshot

	var parseErr error

	scanner := bufio.NewScanner(reader)

	lineNumber := 0
	for scanner.Scan() {
		lineNumber++

		parts := strings.Split(scanner.Text(), "\t")
		if len(parts) != snapshotFieldCount {
			parseErr = errors.Join(parseErr, fmt.Errorf(
				"%w on line %d: got %d fields, want %d",
				errInvalidSnapshotOutput, lineNumber, len(parts), snapshotFieldCount,
			))

			continue
		}

		size, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			parseErr = errors.Join(parseErr, fmt.Errorf(
				"%w on line %d: parse used value: %w", errInvalidSnapshotOutput, lineNumber, err,
			))

			continue
		}

		snapshots = append(snapshots, Snapshot{
			Name: parts[0], Used: size, runner: runner, output: output, state: state, usedKnown: true,
		})
	}

	if err := scanner.Err(); err != nil {
		parseErr = errors.Join(parseErr, fmt.Errorf("scan snapshot output: %w", err))
	}

	if parseErr != nil {
		return nil, parseErr
	}

	return snapshots, nil
}

// CreateSnapshots creates snapshots using the client's command runner.
func (client Client) CreateSnapshots(
	ctx context.Context,
	datasetNames []string,
	snapshotName string,
	recursive bool,
	dbName string,
	dryRun, verbose, debug bool,
) error {
	targets, err := buildSnapshotTargets(datasetNames, snapshotName)
	if err != nil {
		return err
	}

	zfsArgs := []string{"snapshot"}
	if recursive {
		zfsArgs = append(zfsArgs, "-r")
	}

	zfsArgs = append(zfsArgs, targets...)

	switch dbName {
	case "mysql":
		err = client.createMySQLSnapshots(ctx, zfsArgs, dryRun, verbose || debug)
		if err != nil {
			return fmt.Errorf("create snapshots %s: %w", strings.Join(targets, ", "), err)
		}

		return nil

	case "postgresql":
		err = client.createPostgreSQLSnapshots(ctx, zfsArgs, dryRun, verbose || debug)
		if err != nil {
			return fmt.Errorf("create snapshots %s: %w", strings.Join(targets, ", "), err)
		}

		return nil
	}

	if debug || verbose {
		_, _ = fmt.Fprintln(client.output, shellCommand("zfs", zfsArgs...))
	}

	if dryRun {
		return nil
	}

	if err = client.runner.Run(ctx, io.Discard, "zfs", zfsArgs...); err != nil {
		return fmt.Errorf("create snapshots %s: %w", strings.Join(targets, ", "), err)
	}

	return nil
}

func buildSnapshotTargets(datasetNames []string, snapshotName string) ([]string, error) {
	if snapshotName == "" {
		return nil, ErrEmptySnapshotName
	}

	if strings.Contains(snapshotName, "@") {
		return nil, fmt.Errorf("%w: %s", ErrInvalidSnapshotName, snapshotName)
	}

	if len(datasetNames) < 1 {
		return nil, ErrNoDatasets
	}

	targets := make([]string, 0, len(datasetNames))
	for _, datasetName := range datasetNames {
		if datasetName == "" || strings.Contains(datasetName, "@") {
			return nil, fmt.Errorf("%w: %s", ErrInvalidSnapshotName, datasetName)
		}

		targets = append(targets, datasetName+"@"+snapshotName)
	}

	return targets, nil
}

func (client Client) createMySQLSnapshots(
	ctx context.Context,
	zfsArgs []string,
	dryRun, showCommand bool,
) error {
	sql := "FLUSH LOGS; FLUSH TABLES WITH READ LOCK; SYSTEM " + shellCommand("zfs", zfsArgs...) +
		"; UNLOCK TABLES;"

	if showCommand {
		_, _ = fmt.Fprintln(client.output, shellCommand("mysql", "-e", sql))
	}

	if dryRun {
		return nil
	}

	if err := client.runner.Run(ctx, io.Discard, "mysql", "-e", sql); err != nil {
		return fmt.Errorf("create MySQL snapshots: %w", err)
	}

	return nil
}

func (client Client) createPostgreSQLSnapshots(
	ctx context.Context,
	zfsArgs []string,
	dryRun, showCommand bool,
) error {
	startArgs := []string{"-c", "SELECT PG_START_BACKUP('zfs-auto-snapshot');", "postgres"}
	stopArgs := []string{"-c", "SELECT PG_STOP_BACKUP();", "postgres"}

	if showCommand {
		_, _ = fmt.Fprintln(client.output, shellCommand("psql", startArgs...))
		_, _ = fmt.Fprintln(client.output, shellCommand("zfs", zfsArgs...))
		_, _ = fmt.Fprintln(client.output, shellCommand("psql", stopArgs...))
	}

	if dryRun {
		return nil
	}

	var result error

	if err := client.runner.Run(ctx, io.Discard, "psql", startArgs...); err != nil {
		result = errors.Join(result, fmt.Errorf("start PostgreSQL backup: %w", err))
	}

	if err := client.runner.Run(ctx, io.Discard, "zfs", zfsArgs...); err != nil {
		result = errors.Join(result, fmt.Errorf("create PostgreSQL snapshots: %w", err))
	}

	if err := client.runner.Run(ctx, io.Discard, "psql", stopArgs...); err != nil {
		result = errors.Join(result, fmt.Errorf("stop PostgreSQL backup: %w", err))
	}

	return result
}

func shellCommand(name string, args ...string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, shellQuote(name))

	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}

	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}

	for _, char := range value {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-", char) {
			return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
		}
	}

	return value
}

type createOptions struct {
	recursive  bool
	dryRun     bool
	verbose    bool
	debug      bool
	useThreads bool
}

// CreateManySnapshots creates snapshots using the client's command runner.
func (client Client) CreateManySnapshots(
	ctx context.Context,
	snapshotName string,
	datasets []Dataset,
	recursive bool,
	dryRun, verbose, debug, useThreads bool,
) ([]string, error) {
	if err := validateCreateManyRequest(snapshotName, datasets); err != nil {
		return nil, err
	}

	options := createOptions{
		recursive: recursive, dryRun: dryRun, verbose: verbose, debug: debug, useThreads: useThreads,
	}
	dbDatasets, regularDatasets := partitionDatasets(datasets)
	created, result := client.createIndividualSnapshots(ctx, snapshotName, dbDatasets, options)

	if len(regularDatasets) > 0 {
		var regularCreated []string

		var regularErr error

		if client.hasBookmarks(ctx, debug) {
			regularCreated, regularErr = client.createPooledSnapshots(ctx, snapshotName, regularDatasets, options)
		} else {
			regularCreated, regularErr = client.createIndividualSnapshots(ctx, snapshotName, regularDatasets, options)
		}

		created = append(created, regularCreated...)
		result = errors.Join(result, regularErr)
	}

	if result != nil {
		return created, errors.Join(ErrOneSnapshotOfManyErrored, result)
	}

	return created, nil
}

func validateCreateManyRequest(snapshotName string, datasets []Dataset) error {
	if snapshotName == "" {
		return ErrEmptySnapshotName
	}

	if strings.Contains(snapshotName, "@") {
		return fmt.Errorf("%w: %s", ErrInvalidSnapshotName, snapshotName)
	}

	if len(datasets) < 1 {
		return ErrNoDatasets
	}

	for _, v := range datasets {
		if strings.Contains(v.Name, "@") || v.Name == "" {
			return fmt.Errorf("%w: %s", ErrInvalidSnapshotName, v)
		}
	}

	return nil
}

func partitionDatasets(datasets []Dataset) ([]Dataset, []Dataset) {
	var database, regular []Dataset

	for _, dataset := range datasets {
		if dataset.DB != "" {
			database = append(database, dataset)
		} else {
			regular = append(regular, dataset)
		}
	}

	return database, regular
}

func (client Client) createIndividualSnapshots(
	ctx context.Context,
	snapshotName string,
	datasets []Dataset,
	options createOptions,
) ([]string, error) {
	if !options.useThreads {
		var created []string

		var result error

		for _, dataset := range datasets {
			err := client.CreateSnapshots(
				ctx,
				[]string{dataset.Name}, snapshotName, options.recursive, dataset.DB,
				options.dryRun, options.verbose, options.debug,
			)

			result = errors.Join(result, err)
			if err == nil {
				created = append(created, dataset.Name)
			}
		}

		return created, result
	}

	type creationResult struct {
		err  error
		name string
	}

	results := make(chan creationResult, len(datasets))
	for _, dataset := range datasets {
		go func() {
			err := client.CreateSnapshots(
				ctx,
				[]string{dataset.Name}, snapshotName, options.recursive, dataset.DB,
				options.dryRun, options.verbose, options.debug,
			)
			results <- creationResult{name: dataset.Name, err: err}
		}()
	}

	var created []string

	var result error

	for range datasets {
		creation := <-results

		result = errors.Join(result, creation.err)
		if creation.err == nil {
			created = append(created, creation.name)
		}
	}

	return created, result
}

func (client Client) createPooledSnapshots(
	ctx context.Context,
	snapshotName string,
	datasets []Dataset,
	options createOptions,
) ([]string, error) {
	pools := make(map[string][]string)
	maxTargetLength := 0

	for _, dataset := range datasets {
		pool, _, _ := strings.Cut(dataset.Name, "/")
		pools[pool] = append(pools[pool], dataset.Name)
		targetLength := len(dataset.Name) + 1 + len(snapshotName)

		if targetLength > maxTargetLength {
			maxTargetLength = targetLength
		}
	}

	available := max(client.getArgMax(ctx)-argMaxSafetyMargin, 1)
	chunkSize := max(available/maxTargetLength, 1)

	var created []string

	var result error

	for _, datasetNames := range pools {
		for index := 0; index < len(datasetNames); index += chunkSize {
			end := min(index+chunkSize, len(datasetNames))
			err := client.CreateSnapshots(
				ctx,
				datasetNames[index:end], snapshotName, options.recursive, "",
				options.dryRun, options.verbose, options.debug,
			)

			result = errors.Join(result, err)
			if err == nil {
				created = append(created, datasetNames[index:end]...)
			}
		}
	}

	return created, result
}

func (client Client) getArgMax(ctx context.Context) int {
	var out bytes.Buffer

	err := client.runner.Run(ctx, &out, "getconf", "ARG_MAX")
	if err != nil {
		return argMaxFallback
	}

	val, err := strconv.ParseInt(strings.TrimSpace(out.String()), 10, 64)
	if err != nil {
		return argMaxFallback
	}

	return int(val)
}

// DestroySnapshot destroys a snapshot using the client's command runner.
func (client Client) DestroySnapshot(ctx context.Context, name string, dryRun, debug bool) error {
	args := []string{"destroy", "-d", name}

	if debug {
		_, _ = fmt.Fprintln(client.output, "zfs", strings.Join(args, " "))
	}

	var err error

	if !dryRun {
		err = client.runner.Run(ctx, io.Discard, "zfs", args...)
		if err != nil {
			return fmt.Errorf("error destroying snapshot: %w", err)
		}

		client.snapshotState.stale.Store(true)
	}

	return nil
}
