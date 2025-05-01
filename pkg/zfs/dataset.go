package zfs

import (
	"bufio"
	"fmt"
	"strings"
)

type Dataset struct {
	Name       string
	Properties map[string]string
	DB         string
}

// Equals returns true if the other dataset has the same name
func (d Dataset) Equals(other Dataset) bool {
	return d.Name == other.Name
}

// ListDatasets returns a list of ZFS datasets for the pool and properties
func ListDatasets(pool string, properties []string, debug bool) []Dataset {
	var datasets []Dataset

	cmdProperties := append([]string{"name", "type"}, properties...)

	args := []string{"list", "-H", "-t", "filesystem,volume", "-o", strings.Join(cmdProperties, ","), "-s", "name"}
	if pool != "" {
		args = append(args, "-r", pool)
	}

	if debug {
		fmt.Println("zfs " + strings.Join(args, " "))
	}

	cmd := runZfsFn("zfs", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return []Dataset{}
	}

	err = cmd.Start()
	if err != nil {
		return []Dataset{}
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		values := strings.Split(line, "\t")

		if len(values) < 2 {
			continue
		}

		name := values[0]
		typ := values[1]
		props := map[string]string{"type": typ}

		for i, prop := range properties {
			if i+2 >= len(values) {
				continue
			}

			val := values[i+2]
			if val == "-" {
				continue
			}

			props[prop] = val
		}

		dataset := Dataset{Name: name, Properties: props}

		db, ok := props["com.sun:auto-snapshot"]
		if ok {
			if db == "mysql" || db == "postgresql" {
				dataset.DB = db
			}
		}

		datasets = append(datasets, dataset)
	}

	err = cmd.Wait()
	if err != nil {
		return []Dataset{}
	}

	return datasets
}
