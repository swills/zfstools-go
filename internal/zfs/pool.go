package zfs

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
)

type Pool struct {
	Properties map[string]string
	Name       string
}

// ListPools returns ZFS pools using the client's command runner.
func (client Client) ListPools(name string, cmdProps []string, debug bool) ([]Pool, error) {
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
		line := strings.Join(append([]string{"zpool"}, args...), " ")
		if strings.Contains(strings.Join(args, " "), "@") {
			line += " 2>/dev/null"
		}

		_, _ = fmt.Fprintln(client.output, line)
	}

	out, err := client.runner.Run("zpool", args...)
	if err != nil {
		return nil, fmt.Errorf("zpool get: %w", err)
	}

	return parsePools(bytes.NewReader(out)), nil
}

func parsePools(reader io.Reader) []Pool {
	poolProps := map[string]map[string]string{}
	scanner := bufio.NewScanner(reader)

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

	pools := make([]Pool, 0, 1)

	for _, poolName := range slices.Sorted(maps.Keys(poolProps)) {
		pools = append(pools, Pool{
			Name:       poolName,
			Properties: poolProps[poolName],
		})
	}

	return pools
}
