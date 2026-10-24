package configdirector

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ConfigDirector/go-sdks/configdirector/internal/servedvalues"
)

type offlineSource int

const (
	offlineSourceNone offlineSource = iota
	offlineSourceFile
	offlineSourceInlineValues
	offlineSourceFileAndValuesTogether
)

func offlineSourceOf(connection ConnectionOptions) offlineSource {
	switch {
	case connection.File != "" && connection.Values != nil:
		return offlineSourceFileAndValuesTogether
	case connection.File != "":
		return offlineSourceFile
	case connection.Values != nil:
		return offlineSourceInlineValues
	default:
		return offlineSourceNone
	}
}

func offlineConfigsFrom(connection ConnectionOptions) (map[string]servedvalues.Config, error) {
	switch offlineSourceOf(connection) {
	case offlineSourceNone:
		return map[string]servedvalues.Config{}, nil
	case offlineSourceFile:
		return offlineConfigsFromFile(connection.File)
	case offlineSourceInlineValues:
		return servedvalues.ConfigsFrom(connection.Values)
	case offlineSourceFileAndValuesTogether:
		return nil, errors.New("configdirector: both File and Values are given; offline values come from one of them")
	}
	panic("configdirector: an offline source was not decided")
}

func offlineConfigsFromFile(path string) (map[string]servedvalues.Config, error) {
	document, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("configdirector: the offline values file cannot be read: %w", err)
	}

	values, err := valuesObjectFrom(document)
	if err != nil {
		return nil, fmt.Errorf("configdirector: the offline values file %s is not a JSON object with one value per config key: %w", path, err)
	}
	configs, err := servedvalues.ConfigsFrom(values)
	if err != nil {
		return nil, fmt.Errorf("configdirector: the offline values file %s holds a value that cannot be served: %w", path, err)
	}
	return configs, nil
}

func valuesObjectFrom(document []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.UseNumber()

	var values map[string]any
	if err := decoder.Decode(&values); err != nil {
		return nil, err
	}
	if values == nil {
		return nil, errors.New("the document is null")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("the document continues after the object")
	}
	return values, nil
}
