package zfs

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

var staleSnapshotSize = false

type Snapshot struct {
	Name string
	Used int64
}

// GetUsed returns the used size of the snapshot (refreshes if stale)
func (s *Snapshot) GetUsed(debug bool) int64 {
	if s.Used == 0 || staleSnapshotSize {
		if debug {
			fmt.Println("zfs get -Hp -o value used", s.Name)
		}

		cmd := runZfsFn("zfs", "get", "-Hp", "-o", "value", "used", s.Name)

		out, err := cmd.Output()

		if err != nil {
			return 0
		}

		s.Used, _ = strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	}

	return s.Used
}

// IsZero reports if the snapshot is effectively empty
func (s *Snapshot) IsZero(debug bool) bool {
	return s.GetUsed(debug) == 0
}

func toIntPrefix(s string) int64 {
	s = strings.TrimSpace(s)
	digits := ""

	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits += string(r)
		} else {
			break
		}
	}

	if digits == "" {
		return 0
	}

	val, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0
	}

	return val
}

// ListSnapshots returns all snapshots, optionally recursive
func ListSnapshots(dataset string, recursive bool, debug bool) ([]Snapshot, error) {
	args := []string{"list"}

	if dataset != "" && !recursive {
		args = append(args, "-d", "1")
	}

	if recursive {
		args = append(args, "-r")
	}

	args = append(args, "-H", "-t", "snapshot", "-o", "name,used", "-S", "name")

	if dataset != "" {
		args = append(args, dataset)
	}

	if debug {
		fmt.Println("zfs", strings.Join(args, " "))
	}

	cmd := runZfsFn("zfs", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("error creating StdoutPipe: %w", err)
	}

	err = cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("error starting command: %w", err)
	}

	var snapshots []Snapshot

	scanner := bufio.NewScanner(stdout)

	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), "\t")
		if len(parts) != 2 {
			continue
		}

		size := toIntPrefix(parts[1])
		snapshots = append(snapshots, Snapshot{Name: parts[0], Used: size})
	}

	err = cmd.Wait()
	if err != nil {
		return nil, fmt.Errorf("error waiting on command: %w", err)
	}

	return snapshots, nil
}

// Create creates a single snapshot or a group of snapshots
func Create(targets []string, recursive bool, dbName string, dryRun, verbose, debug bool) {
	base := []string{"zfs", "snapshot"}
	if recursive {
		base = append(base, "-r")
	} else {
		base = append(base, "")
	}

	cmdLine := base
	cmdLine = append(cmdLine, targets...)

	cmdStr := strings.Join(cmdLine, " ")
	if dbName == "mysql" {
		sql := fmt.Sprintf(`
FLUSH LOGS;
FLUSH TABLES WITH READ LOCK;
SYSTEM %s;
UNLOCK TABLES;`, cmdStr)
		cmdStr = fmt.Sprintf(`mysql -e "%s"`, strings.ReplaceAll(sql, "\n", " "))
	} else if dbName == "postgresql" {
		cmdStr = fmt.Sprintf(`(psql -c "SELECT PG_START_BACKUP('zfs-auto-snapshot');" postgres ; %s ) ; psql -c "SELECT PG_STOP_BACKUP();" postgres`, cmdStr)
	}

	if debug || verbose {
		fmt.Println(cmdStr)
	}

	if !dryRun {
		_ = runZfsFn("sh", "-c", cmdStr).Run()
	}
}

// CreateMany handles parallel and multi-snapshot creation
func CreateMany(snapshotName string, datasets []Dataset, recursive bool, dryRun, verbose, debug, useThreads bool) {
	if len(datasets) == 0 {
		return
	}

	// Split out DB datasets
	var dbDatasets []Dataset

	var regular []Dataset

	for _, ds := range datasets {
		if ds.DB != "" {
			dbDatasets = append(dbDatasets, ds)
		} else {
			regular = append(regular, ds)
		}
	}

	if len(dbDatasets) > 0 {
		CreateMany(snapshotName, dbDatasets, recursive, dryRun, verbose, debug, useThreads)
	}

	// If multi-snapshot is supported, use pooled batching
	if HasMultiSnap(debug) {
		var snapshots []string

		maxLen := 0

		for _, ds := range regular {
			snap := fmt.Sprintf("%s@%s", ds.Name, snapshotName)

			snapshots = append(snapshots, snap)

			if len(snap) > maxLen {
				maxLen = len(snap)
			}
		}

		argMax := getArgMax()
		argMax -= 1024 // safety slack
		chunkSize := argMax / maxLen

		// group by pool
		pools := make(map[string][]string)

		for _, snap := range snapshots {
			parts := strings.SplitN(snap, "@", 2)
			pool := strings.SplitN(parts[0], "/", 2)[0]
			pools[pool] = append(pools[pool], snap)
		}

		for _, snaps := range pools {
			for index := 0; index < len(snaps); index += chunkSize {
				end := index + chunkSize

				if end > len(snaps) {
					end = len(snaps)
				}

				Create(snaps[index:end], recursive, "", dryRun, verbose, debug)
			}
		}

		return
	}

	// fallback: serial or threaded single snapshot
	var waitGroup sync.WaitGroup

	for _, ds := range regular {
		snap := fmt.Sprintf("%s@%s", ds.Name, snapshotName)
		dbName := ds.DB

		waitGroup.Add(1)

		go func(name, db string) {
			defer waitGroup.Done()
			Create([]string{name}, recursive, db, dryRun, verbose, debug)
		}(snap, dbName)

		if !useThreads {
			waitGroup.Wait()
		}
	}

	waitGroup.Wait()
}

func getArgMax() int {
	out, err := runZfsFn("getconf", "ARG_MAX").Output()
	if err != nil {
		return 4096 // conservative fallback
	}

	val, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 4096
	}

	return val
}

// DestroySnapshot deletes a snapshot (and marks usage as stale)
func DestroySnapshot(name string, dryRun, debug bool) {
	staleSnapshotSize = true
	args := []string{"destroy", "-d"}

	args = append(args, name)

	if debug {
		fmt.Println("zfs", strings.Join(args, " "))
	}

	if !dryRun {
		_ = runZfsFn("zfs", args...).Run()
	}
}
