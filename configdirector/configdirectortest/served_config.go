package configdirectortest

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/ConfigDirector/go-sdks/configdirector"
)

const payloadVersion = 1

type payloadKind string

const (
	payloadKindFull  payloadKind = "full"
	payloadKindDelta payloadKind = "delta"
)

type servedConfig struct {
	configType configdirector.ConfigType
	text       string
}

func servedConfigsFrom(values map[string]any) (map[string]servedConfig, error) {
	configs := make(map[string]servedConfig, len(values))
	for key, value := range values {
		config, err := servedConfigFrom(key, value)
		if err != nil {
			return nil, err
		}
		configs[key] = config
	}
	return configs, nil
}

func servedConfigFrom(key string, value any) (servedConfig, error) {
	if strings.TrimSpace(key) == "" {
		return servedConfig{}, fmt.Errorf("configdirectortest: the key %q is blank; a value needs a config key", key)
	}
	if value == nil {
		return servedConfig{}, fmt.Errorf("configdirectortest: the value for %q is nil, which is not a value; use RemoveValue to remove a value", key)
	}

	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Bool:
		return servedConfig{configType: configdirector.ConfigTypeBoolean, text: strconv.FormatBool(reflected.Bool())}, nil
	case reflect.String:
		return servedConfig{configType: configdirector.ConfigTypeString, text: reflected.String()}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return servedConfig{configType: configdirector.ConfigTypeInteger, text: strconv.FormatInt(reflected.Int(), 10)}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return servedConfig{configType: configdirector.ConfigTypeInteger, text: strconv.FormatUint(reflected.Uint(), 10)}, nil
	case reflect.Float32, reflect.Float64:
		return floatConfigFrom(key, reflected)
	case reflect.Map, reflect.Slice, reflect.Array, reflect.Struct:
		return jsonConfigFrom(key, value)
	default:
		return servedConfig{}, fmt.Errorf(
			"configdirectortest: the value for %q is a %T, which cannot be served; a test value is a bool, a string, an integer, a float, or a map, slice, array or struct that encodes as JSON",
			key, value,
		)
	}
}

func floatConfigFrom(key string, reflected reflect.Value) (servedConfig, error) {
	number := reflected.Float()
	if math.IsNaN(number) || math.IsInf(number, 0) {
		return servedConfig{}, fmt.Errorf("configdirectortest: the value for %q is %v, which is not a finite number", key, number)
	}
	return servedConfig{configType: configdirector.ConfigTypeFloat, text: strconv.FormatFloat(number, 'f', -1, reflected.Type().Bits())}, nil
}

func jsonConfigFrom(key string, value any) (servedConfig, error) {
	document, err := json.Marshal(value)
	if err != nil {
		return servedConfig{}, fmt.Errorf("configdirectortest: the value for %q does not encode as JSON: %w", key, err)
	}
	return servedConfig{configType: configdirector.ConfigTypeJSON, text: string(document)}, nil
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

func fullDocument(configs map[string]servedConfig) string {
	return documentOf(payloadKindFull, configs)
}

func deltaDocument(key string, config servedConfig) string {
	return documentOf(payloadKindDelta, map[string]servedConfig{key: config})
}

func documentOf(kind payloadKind, configs map[string]servedConfig) string {
	payload := wirePayload{
		PayloadVersion: payloadVersion,
		Kind:           kind,
		Configs:        make(map[string]wireConfig, len(configs)),
		Segments:       map[string]any{},
	}
	for key, config := range configs {
		payload.Configs[key] = wireConfig{
			ID:   "test-config:" + key,
			Key:  key,
			Type: string(config.configType),
			Target: wireTarget{
				Rules:          []any{},
				DefaultValue:   config.text,
				DefaultValueID: "test-value:" + key,
			},
		}
	}
	document, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("configdirectortest: the payload cannot be encoded: %v", err))
	}
	return string(document)
}
