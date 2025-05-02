package zfs

import (
	"bufio"
	"fmt"
	"maps"
	"slices"
	"strings"
)

type Pool struct {
	Properties map[string]string
	Name       string
}

func ListPools(name string, cmdProps []string, debug bool) ([]Pool, error) {
	if len(cmdProps) == 0 {
		cmdProps = []string{"all"}
	}

	args := []string{
		"get", "-H", "-p", "-o", "name,property,value", strings.Join(cmdProps, ","),
	}

	if name != "" {
		args = append(args, name)
	}

	if debug {
		fmt.Printf("zpool " + strings.Join(args, " ")) //nolint:forbidigo

		if strings.Contains(strings.Join(args, " "), "@") {
			fmt.Print(" 2>/dev/null") //nolint:forbidigo
		}

		fmt.Printf("\n") //nolint:forbidigo
	}

	cmd := runZpoolFn("zpool", args...)
	stdout, err := cmd.StdoutPipe()

	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	err = cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("zpool start: %w", err)
	}

	poolProps := map[string]map[string]string{}

	scanner := bufio.NewScanner(stdout)

	for scanner.Scan() {
		line := scanner.Text()
		values := strings.Split(line, "\t")

		if len(values) < 3 {
			continue
		}

		poolName, propName, propValue := values[0], values[1], values[2]

		_, ok := poolProps[poolName]
		if !ok {
			poolProps[poolName] = map[string]string{}
		}

		poolProps[poolName][propName] = propValue
	}

	err = cmd.Wait()
	if err != nil {
		return nil, fmt.Errorf("zpool wait: %w", err)
	}

	pools := make([]Pool, 0, 1)

	for _, poolName := range slices.Sorted(maps.Keys(poolProps)) {
		pools = append(pools, Pool{
			Name:       poolName,
			Properties: poolProps[poolName],
		})
	}

	return pools, nil
}
