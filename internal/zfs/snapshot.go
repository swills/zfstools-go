package zfs

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

var ErrEmptySnapshotName = errors.New("empty snapshot name")

var ErrInvalidSnapshotName = errors.New("invalid snapshot name")

var ErrNoDatasets = errors.New("no dataset(s) specified")

var ErrOneSnapshotOfManyErrored = errors.New("some snapshots failed to create")

type Snapshot struct {
	runner Runner
	output io.Writer
	state  *snapshotState
	Name   string
	Used   int64
}

// GetUsed returns the used size of the snapshot (refreshes if stale)
func (s *Snapshot) GetUsed(debug bool) int64 {
	runner, output, state := s.runner, s.output, s.state
	if runner == nil {
		client := NewSystemClient(io.Discard)
		runner, output, state = client.runner, client.output, client.snapshotState
	}

	if s.Used == 0 || state.stale.Load() {
		if debug {
			_, _ = fmt.Fprintln(output, "zfs get -Hp -o value used", s.Name)
		}

		out, err := runner.Run("zfs", "get", "-Hp", "-o", "value", "used", s.Name)
		if err != nil {
			return 0
		}

		s.Used, err = strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		if err != nil {
			s.Used = 0
		}
	}

	return s.Used
}

// IsZero reports if the snapshot is effectively empty
func (s *Snapshot) IsZero(debug bool) bool {
	return s.GetUsed(debug) == 0
}

// ListSnapshots returns snapshots using the client's command runner.
func (client Client) ListSnapshots(dataset string, recursive bool, debug bool) ([]Snapshot, error) {
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

	out, err := client.runner.Run("zfs", args...)
	if err != nil {
		return nil, fmt.Errorf("error listing snapshots: %w", err)
	}

	return parseSnapshots(bytes.NewReader(out), client.runner, client.output, client.snapshotState), nil
}

func parseSnapshots(reader io.Reader, runner Runner, output io.Writer, state *snapshotState) []Snapshot {
	snapshots := []Snapshot{}
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), "\t")
		if len(parts) != 2 {
			continue
		}

		size, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}

		snapshots = append(snapshots, Snapshot{
			Name: parts[0], Used: size, runner: runner, output: output, state: state,
		})
	}

	return snapshots
}

// CreateSnapshots creates snapshots using the client's command runner.
func (client Client) CreateSnapshots(
	datasetNames []string,
	snapshotName string,
	recursive bool,
	dbName string,
	dryRun, verbose, debug bool,
) error {
	if snapshotName == "" {
		return ErrEmptySnapshotName
	}

	if strings.Contains(snapshotName, "@") {
		return fmt.Errorf("%w: %s", ErrInvalidSnapshotName, snapshotName)
	}

	if len(datasetNames) < 1 {
		return ErrNoDatasets
	}

	targets := make([]string, 0, len(datasetNames))
	for _, datasetName := range datasetNames {
		if datasetName == "" || strings.Contains(datasetName, "@") {
			return fmt.Errorf("%w: %s", ErrInvalidSnapshotName, datasetName)
		}

		targets = append(targets, datasetName+"@"+snapshotName)
	}

	base := []string{"zfs", "snapshot"}
	if recursive {
		base = append(base, "-r")
	}

	cmdLine := base
	cmdLine = append(cmdLine, targets...)

	cmdStr := strings.Join(cmdLine, " ")

	switch dbName {
	case "mysql":
		sql := fmt.Sprintf(`
FLUSH LOGS;
FLUSH TABLES WITH READ LOCK;
SYSTEM %s;
UNLOCK TABLES;`, cmdStr)
		cmdStr = fmt.Sprintf(`mysql -e "%s"`, strings.ReplaceAll(sql, "\n", " "))

	case "postgresql":
		cmdStr = fmt.Sprintf(
			`(psql -c "SELECT PG_START_BACKUP('zfs-auto-snapshot');" postgres ; %s ) ; `+
				`psql -c "SELECT PG_STOP_BACKUP();" postgres`,
			cmdStr,
		)
	}

	if debug || verbose {
		_, _ = fmt.Fprintln(client.output, cmdStr)
	}

	var err error

	if !dryRun {
		_, err = client.runner.Run("sh", "-c", cmdStr)
		if err != nil {
			return fmt.Errorf("error creating snapshot: %w", err)
		}
	}

	return nil
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
	snapshotName string,
	datasets []Dataset,
	recursive bool,
	dryRun, verbose, debug, useThreads bool,
) error {
	if err := validateCreateManyRequest(snapshotName, datasets); err != nil {
		return err
	}

	options := createOptions{
		recursive: recursive, dryRun: dryRun, verbose: verbose, debug: debug, useThreads: useThreads,
	}
	dbDatasets, regularDatasets := partitionDatasets(datasets)
	failed := client.createIndividualSnapshots(snapshotName, dbDatasets, options)

	if len(regularDatasets) > 0 {
		if client.hasBookmarks(debug) {
			failed = client.createPooledSnapshots(snapshotName, regularDatasets, options) || failed
		} else {
			failed = client.createIndividualSnapshots(snapshotName, regularDatasets, options) || failed
		}
	}

	if failed {
		return ErrOneSnapshotOfManyErrored
	}

	return nil
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
	snapshotName string,
	datasets []Dataset,
	options createOptions,
) bool {
	if !options.useThreads {
		failed := false

		for _, dataset := range datasets {
			if client.CreateSnapshots(
				[]string{dataset.Name}, snapshotName, options.recursive, dataset.DB,
				options.dryRun, options.verbose, options.debug,
			) != nil {
				failed = true
			}
		}

		return failed
	}

	results := make(chan error, len(datasets))
	for _, dataset := range datasets {
		go func() {
			results <- client.CreateSnapshots(
				[]string{dataset.Name}, snapshotName, options.recursive, dataset.DB,
				options.dryRun, options.verbose, options.debug,
			)
		}()
	}

	failed := false

	for range datasets {
		if <-results != nil {
			failed = true
		}
	}

	return failed
}

func (client Client) createPooledSnapshots(
	snapshotName string,
	datasets []Dataset,
	options createOptions,
) bool {
	pools := make(map[string][]string)
	maxTargetLength := 0

	for _, dataset := range datasets {
		pool := strings.SplitN(dataset.Name, "/", 2)[0]
		pools[pool] = append(pools[pool], dataset.Name)
		targetLength := len(dataset.Name) + 1 + len(snapshotName)

		if targetLength > maxTargetLength {
			maxTargetLength = targetLength
		}
	}

	available := max(client.getArgMax()-1024, 1)
	chunkSize := max(available/maxTargetLength, 1)
	failed := false

	for _, datasetNames := range pools {
		for index := 0; index < len(datasetNames); index += chunkSize {
			end := min(index+chunkSize, len(datasetNames))
			if client.CreateSnapshots(
				datasetNames[index:end], snapshotName, options.recursive, "",
				options.dryRun, options.verbose, options.debug,
			) != nil {
				failed = true
			}
		}
	}

	return failed
}

func (client Client) getArgMax() int {
	var err error

	var out []byte

	var val int64

	out, err = client.runner.Run("getconf", "ARG_MAX")
	if err != nil {
		return 4096 // conservative fallback
	}

	val, err = strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 4096
	}

	return int(val)
}

// DestroySnapshot destroys a snapshot using the client's command runner.
func (client Client) DestroySnapshot(name string, dryRun, debug bool) error {
	client.snapshotState.stale.Store(true)

	args := []string{"destroy", "-d", name}

	if debug {
		_, _ = fmt.Fprintln(client.output, "zfs", strings.Join(args, " "))
	}

	var err error

	if !dryRun {
		_, err = client.runner.Run("zfs", args...)
		if err != nil {
			return fmt.Errorf("error destroying snapshot: %w", err)
		}
	}

	return nil
}
