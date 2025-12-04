// Package config provides internal configuration loading and processing.
package config

import (
	"reflect"
	"time"

	"github.com/go-viper/mapstructure/v2"

	"github.com/smykla-labs/klaudiush/pkg/config"
)

// CustomDecoderConfig returns a mapstructure decoder config with custom type hooks
// for handling Duration and Severity types.
func CustomDecoderConfig() *mapstructure.DecoderConfig {
	return &mapstructure.DecoderConfig{
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			stringToDurationHookFunc(),
			stringToSeverityHookFunc(),
			mapstructure.StringToTimeDurationHookFunc(),
		),
		WeaklyTypedInput: true,
		Result:           nil, // Set by caller
	}
}

// stringToDurationHookFunc returns a decode hook for converting strings to config.Duration.
//
//nolint:ireturn // required by mapstructure.DecodeHookFunc interface
func stringToDurationHookFunc() mapstructure.DecodeHookFunc {
	return func(
		_ reflect.Type,
		t reflect.Type,
		data any,
	) (any, error) {
		if t != reflect.TypeFor[config.Duration]() {
			return data, nil
		}

		switch v := data.(type) {
		case string:
			d, err := time.ParseDuration(v)
			if err != nil {
				return nil, err
			}

			return config.Duration(d), nil

		case int64:
			return config.Duration(time.Duration(v)), nil

		case float64:
			return config.Duration(time.Duration(v)), nil

		default:
			return data, nil
		}
	}
}

// stringToSeverityHookFunc returns a decode hook for converting strings to config.Severity.
//
//nolint:ireturn // required by mapstructure.DecodeHookFunc interface
func stringToSeverityHookFunc() mapstructure.DecodeHookFunc {
	return func(
		_ reflect.Type,
		t reflect.Type,
		data any,
	) (any, error) {
		if t != reflect.TypeFor[config.Severity]() {
			return data, nil
		}

		switch v := data.(type) {
		case string:
			return config.SeverityString(v)

		case int:
			return config.Severity(v), nil

		case int64:
			return config.Severity(v), nil

		default:
			return data, nil
		}
	}
}
