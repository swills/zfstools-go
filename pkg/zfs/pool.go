package zfs

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
)

type Pool struct {
	Properties map[string]string
	Name       string
}

func (p *Pool) Equals(other *Pool) bool {
	return p == other || (other != nil && p.Name == other.Name)
}

func (p *Pool) String() string {
	var out strings.Builder
	for k, v := range p.Properties {
		_, _ = fmt.Fprintf(&out, "[%s] %s: %s\n", p.Name, k, v)
	}

	return out.String()
}

var runZpoolFn = exec.Command

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
		fmt.Printf("zpool " + strings.Join(args, " "))

		if strings.Contains(strings.Join(args, " "), "@") {
			fmt.Print(" 2>/dev/null")
		}

		fmt.Printf("\n")
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
		parts := strings.SplitN(line, "\t", 3)

		if len(parts) < 3 {
			continue
		}

		name, property, value := parts[0], parts[1], parts[2]

		_, ok := poolProps[name]
		if !ok {
			poolProps[name] = map[string]string{}
		}

		poolProps[name][property] = value
	}

	err = cmd.Wait()
	if err != nil {
		return nil, fmt.Errorf("zpool wait: %w", err)
	}

	pools := make([]Pool, 0, len(poolProps))

	for name, props := range poolProps {
		pools = append(pools, Pool{
			Name:       name,
			Properties: props,
		})
	}

	return pools, nil
}
