// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddprofiledefinition

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// symbolContext selects the fields allowed at each symbol location.
type symbolContext int

const (
	scalarSymbol symbolContext = iota
	columnSymbol
	metricTagSymbol
	metadataSymbol
	topologyScalarSymbol
	topologyColumnSymbol
)

func oidHasPrefix(oid, prefix string) bool {
	return oid == prefix || strings.HasPrefix(oid, prefix+".")
}

func validateEnrichSymbol(symbol *SymbolConfig, context symbolContext) error {
	var errs []error

	if symbol.Name == "" {
		errs = append(errs, fmt.Errorf("symbol name missing: name=`%s` oid=`%s`", symbol.Name, symbol.OID))
	}
	if symbol.OID == "" {
		errs = append(errs, fmt.Errorf("symbol oid missing: name=`%s` oid=`%s`", symbol.Name, symbol.OID))
	}
	errs = append(errs, compileSymbolPatterns(symbol))
	errs = append(errs, validateMapping(symbol.Mapping, context))
	if symbol.Mapping.EffectiveMode() == MappingModeBitmask && symbol.Mapping.HasItems() && symbol.ScaleFactor != 0 {
		errs = append(errs, errors.New("`scale_factor` cannot be used with `mapping.mode: bitmask`"))
	}
	if (context != columnSymbol && context != scalarSymbol) && symbol.MetricType != "" {
		errs = append(
			errs,
			errors.New("`metric_type` cannot be used outside scalar/table metric symbols and metrics root"),
		)
	}

	return errors.Join(errs...)
}

func validateMapping(mapping MappingConfig, context symbolContext) error {
	if !mapping.HasItems() {
		if mapping.Mode != "" {
			return errors.New("`mapping.mode` requires `mapping.items`")
		}
		return nil
	}

	var errs []error

	switch mapping.EffectiveMode() {
	case MappingModeExact:
	case MappingModeBitmask:
		if context != scalarSymbol && context != columnSymbol {
			errs = append(errs, errors.New("`mapping.mode: bitmask` is only supported for scalar/table metric symbols"))
		}
		for key, value := range mapping.Items {
			bit, err := strconv.ParseUint(key, 10, 64)
			if err != nil || (bit != 0 && bit&(bit-1) != 0) {
				errs = append(
					errs,
					fmt.Errorf(
						"`mapping.mode: bitmask` requires keys to be 0 or a single power-of-two bit, got %q",
						key,
					),
				)
			}
			if value == "" {
				errs = append(
					errs,
					fmt.Errorf("`mapping.mode: bitmask` requires non-empty values, got empty value for key %q", key),
				)
			}
		}
	default:
		errs = append(errs, fmt.Errorf("invalid `mapping.mode` %q", mapping.Mode))
	}

	return errors.Join(errs...)
}

func compileSymbolPatterns(symbol *SymbolConfig) error {
	var errs []error
	if symbol.ExtractValue != "" {
		pattern, err := regexp.Compile(symbol.ExtractValue)
		if err != nil {
			errs = append(errs, fmt.Errorf("cannot compile `extract_value` (%s): %s", symbol.ExtractValue, err.Error()))
		} else {
			symbol.ExtractValueCompiled = pattern
		}
	}
	if symbol.MatchPattern != "" {
		pattern, err := regexp.Compile(symbol.MatchPattern)
		if err != nil {
			errs = append(errs, fmt.Errorf("cannot compile `match_pattern` (%s): %s", symbol.MatchPattern, err.Error()))
		} else {
			symbol.MatchPatternCompiled = pattern
		}
	}
	return errors.Join(errs...)
}

// isSymbolConfigured distinguishes an absent symbol from an incomplete declaration.
func isSymbolConfigured(symbol SymbolConfig) bool {
	return !reflect.ValueOf(symbol).IsZero()
}

func withValidationPath(path string, err error) error {
	if err == nil || path == "" {
		return err
	}
	return fmt.Errorf("%s: %w", path, err)
}
