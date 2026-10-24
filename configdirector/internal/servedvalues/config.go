// Package servedvalues turns Go values into the config state a client is served without a
// server: one config per key, the same value for every context, delivered as server SDK payload
// documents through an in-memory connection. Package configdirectortest builds its test client on
// it.
package servedvalues

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
)

const payloadVersion = 1

type payloadKind string

const (
	payloadKindFull  payloadKind = "full"
	payloadKindDelta payloadKind = "delta"
)

const (
	configTypeBoolean = "boolean"
	configTypeString  = "string"
	configTypeInteger = "integer"
	configTypeFloat   = "float"
	configTypeJSON    = "json"
)

// Config is one served config: its type and the text of its value, as the payload carries them.
type Config struct {
	configType string
	text       string
}

// ConfigsFrom turns values, keyed by config key, into the configs a connection serves. It returns
// the error of the first value that cannot be served.
func ConfigsFrom(values map[string]any) (map[string]Config, error) {
	configs := make(map[string]Config, len(values))
	for key, value := range values {
		config, err := configFrom(key, value)
		if err != nil {
			return nil, err
		}
		configs[key] = config
	}
	return configs, nil
}

func configFrom(key string, value any) (Config, error) {
	if strings.TrimSpace(key) == "" {
		return Config{}, fmt.Errorf("configdirector: the key %q is blank; a value needs a config key", key)
	}
	if value == nil {
		return Config{}, fmt.Errorf("configdirector: the value for %q is nil, which is not a value; a config holds no null", key)
	}
	if number, isJSONNumber := value.(json.Number); isJSONNumber {
		return numberConfigFrom(key, number)
	}

	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Bool:
		return Config{configType: configTypeBoolean, text: strconv.FormatBool(reflected.Bool())}, nil
	case reflect.String:
		return Config{configType: configTypeString, text: reflected.String()}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return Config{configType: configTypeInteger, text: strconv.FormatInt(reflected.Int(), 10)}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return Config{configType: configTypeInteger, text: strconv.FormatUint(reflected.Uint(), 10)}, nil
	case reflect.Float32, reflect.Float64:
		return floatConfigFrom(key, reflected)
	case reflect.Map, reflect.Slice, reflect.Array, reflect.Struct:
		return jsonConfigFrom(key, value)
	default:
		return Config{}, fmt.Errorf(
			"configdirector: the value for %q is a %T, which cannot be served; a served value is a bool, a string, an integer, a float, or a map, slice, array or struct that encodes as JSON",
			key, value,
		)
	}
}

func numberConfigFrom(key string, number json.Number) (Config, error) {
	if whole, err := number.Int64(); err == nil {
		return Config{configType: configTypeInteger, text: strconv.FormatInt(whole, 10)}, nil
	}
	asFloat, err := number.Float64()
	if err != nil {
		return Config{}, fmt.Errorf("configdirector: the value for %q is %s, which is not a finite number", key, number)
	}
	if asFloat == math.Trunc(asFloat) && asFloat >= math.MinInt64 && asFloat < math.MaxInt64 {
		return Config{configType: configTypeInteger, text: strconv.FormatInt(int64(asFloat), 10)}, nil
	}
	return Config{configType: configTypeFloat, text: strconv.FormatFloat(asFloat, 'f', -1, 64)}, nil
}

func floatConfigFrom(key string, reflected reflect.Value) (Config, error) {
	number := reflected.Float()
	if math.IsNaN(number) || math.IsInf(number, 0) {
		return Config{}, fmt.Errorf("configdirector: the value for %q is %v, which is not a finite number", key, number)
	}
	return Config{configType: configTypeFloat, text: strconv.FormatFloat(number, 'f', -1, reflected.Type().Bits())}, nil
}

func jsonConfigFrom(key string, value any) (Config, error) {
	document, err := json.Marshal(value)
	if err != nil {
		return Config{}, fmt.Errorf("configdirector: the value for %q does not encode as JSON: %w", key, err)
	}
	return Config{configType: configTypeJSON, text: string(document)}, nil
}

type wirePayload struct {
	PayloadVersion int                   `json:"payloadVersion"`
	Kind           payloadKind           `json:"kind"`
	Configs        map[string]wireConfig `json:"configs"`
	Segments       map[string]any        `json:"segments"`
}

type wireConfig struct {
	ID     string     `json:"id"`
	Key    string     `json:"key"`
	Type   string     `json:"type"`
	Target wireTarget `json:"target"`
}

type wireTarget struct {
	Rules          []any  `json:"rules"`
	DefaultValue   string `json:"defaultValue"`
	DefaultValueID string `json:"defaultValueId"`
}

// Origin is where the values a connection serves come from, which decides the config and value
// identifiers the client reports for them.
type Origin int

const (
	// TestValues are the values a configdirectortest.TestClient serves: config test-config:<key>
	// with value test-value:<key>.
	TestValues Origin = iota

	// OfflineValues are the values a client in offline mode serves: config offline:<key> with no
	// value identifier.
	OfflineValues
)

type identifiers struct {
	configID string
	valueID  string
}

func (origin Origin) identifiersOf(key string) identifiers {
	switch origin {
	case TestValues:
		return identifiers{configID: "test-config:" + key, valueID: "test-value:" + key}
	case OfflineValues:
		return identifiers{configID: "offline:" + key, valueID: ""}
	}
	panic(fmt.Sprintf("servedvalues: the origin %d has no identifiers", origin))
}

func (origin Origin) fullDocument(configs map[string]Config) string {
	return origin.documentOf(payloadKindFull, configs)
}

func (origin Origin) deltaDocument(key string, config Config) string {
	return origin.documentOf(payloadKindDelta, map[string]Config{key: config})
}

func (origin Origin) documentOf(kind payloadKind, configs map[string]Config) string {
	payload := wirePayload{
		PayloadVersion: payloadVersion,
		Kind:           kind,
		Configs:        make(map[string]wireConfig, len(configs)),
		Segments:       map[string]any{},
	}
	for key, config := range configs {
		identifiers := origin.identifiersOf(key)
		payload.Configs[key] = wireConfig{
			ID:   identifiers.configID,
			Key:  key,
			Type: config.configType,
			Target: wireTarget{
				Rules:          []any{},
				DefaultValue:   config.text,
				DefaultValueID: identifiers.valueID,
			},
		}
	}
	document, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("servedvalues: the payload cannot be encoded: %v", err))
	}
	return string(document)
}
