package zfs

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

type Dataset struct {
	Name       string
	Properties map[string]string
	DB         string
}

// ListDatasets returns a list of ZFS datasets using the client's command executor.
func (client Client) ListDatasets(pool string, properties []string, debug bool) []Dataset {
	cmdProperties := append([]string{"name", "type"}, properties...)

	args := []string{"list", "-H", "-t", "filesystem,volume", "-o", strings.Join(cmdProperties, ","), "-s", "name"}
	if pool != "" {
		args = append(args, "-r", pool)
	}

	if debug {
		_, _ = fmt.Fprintln(client.output, "zfs "+strings.Join(args, " "))
	}

	reader, done := client.stream("zfs", args...)
	datasets := parseDatasets(reader, properties)
	_ = reader.Close()

	err := <-done
	if err != nil {
		return nil
	}

	return datasets
}

func parseDatasets(reader io.Reader, properties []string) []Dataset {
	var datasets []Dataset

	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		values := strings.Split(scanner.Text(), "\t")

		if len(values) != len(properties)+2 {
			continue
		}

		name := values[0]
		values = values[1:] // emulate Ruby .shift
		props := map[string]string{"type": values[0]}
		values = values[1:] // emulate Ruby .shift

		for i, prop := range properties {
			value := values[i]
			if value == "-" {
				continue
			}

			props[prop] = value
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

	return datasets
}
