package zfs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

const datasetMetadataFieldCount = 2

var errInvalidDatasetOutput = errors.New("invalid zfs dataset output")

type Dataset struct {
	Name       string
	Properties map[string]string
	DB         string
}

// ListDatasets returns ZFS datasets using the client's command runner. It
// returns an error rather than exposing incomplete discovery results.
func (client Client) ListDatasets(
	ctx context.Context,
	pool string,
	properties []string,
	debug bool,
) ([]Dataset, error) {
	cmdProperties := append([]string{"name", "type"}, properties...)

	args := []string{"list", "-H", "-t", "filesystem,volume", "-o", strings.Join(cmdProperties, ","), "-s", "name"}
	if pool != "" {
		args = append(args, "-r", pool)
	}

	if debug {
		_, _ = fmt.Fprintln(client.output, "zfs "+strings.Join(args, " "))
	}

	reader, done := client.stream(ctx, "zfs", args...)
	datasets, parseErr := parseDatasets(reader, properties)
	_ = reader.Close()

	runErr := <-done
	if runErr != nil {
		runErr = fmt.Errorf("list datasets: %w", runErr)
	}

	if err := errors.Join(parseErr, runErr); err != nil {
		return nil, err
	}

	return datasets, nil
}

func parseDatasets(reader io.Reader, properties []string) ([]Dataset, error) {
	var datasets []Dataset

	var parseErr error

	scanner := bufio.NewScanner(reader)

	lineNumber := 0
	for scanner.Scan() {
		lineNumber++

		values := strings.Split(scanner.Text(), "\t")
		wantFields := len(properties) + datasetMetadataFieldCount

		if len(values) != wantFields {
			parseErr = errors.Join(parseErr, fmt.Errorf(
				"%w on line %d: got %d fields, want %d",
				errInvalidDatasetOutput, lineNumber, len(values), wantFields,
			))

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

	if err := scanner.Err(); err != nil {
		parseErr = errors.Join(parseErr, fmt.Errorf("scan dataset output: %w", err))
	}

	if parseErr != nil {
		return nil, parseErr
	}

	return datasets, nil
}
