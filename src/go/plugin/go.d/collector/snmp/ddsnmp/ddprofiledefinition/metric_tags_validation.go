// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddprofiledefinition

import (
	"errors"
	"fmt"
	"regexp"
)

func validateEnrichGlobalMetricTags(metricTags []GlobalMetricTagConfig) error {
	var errs []error
	for i := range metricTags {
		errs = append(errs, validateEnrichMetricTag(&metricTags[i].MetricTagConfig))
		errs = append(errs, validateConsumers(fmt.Sprintf("metric_tags[%d].consumers", i), metricTags[i].Consumers))
	}
	return errors.Join(errs...)
}

func validateEnrichMetricTag(metricTag *MetricTagConfig) error {
	var errs []error

	if (metricTag.Column.OID != "" || metricTag.Column.Name != "") &&
		(metricTag.Symbol.OID != "" || metricTag.Symbol.Name != "") {
		errs = append(
			errs,
			fmt.Errorf(
				"metric tag symbol and column cannot be both declared: symbol=%v, column=%v",
				metricTag.Symbol,
				metricTag.Column,
			),
		)
	}

	// Move deprecated metricTag.Column to metricTag.Symbol
	if metricTag.Column.OID != "" || metricTag.Column.Name != "" {
		metricTag.Symbol = SymbolConfigCompat(metricTag.Column)
		metricTag.Column = SymbolConfig{}
	}

	// OID/Name to Symbol harmonization:
	// When users declare metric tag like:
	//   metric_tags:
	//     - OID: 1.2.3
	//       symbol: aSymbol
	// this will lead to OID stored as MetricTagConfig.OID  and name stored as MetricTagConfig.Symbol.Name
	// When this happens, we harmonize by moving MetricTagConfig.OID to MetricTagConfig.Symbol.OID.
	if metricTag.OID != "" && metricTag.Symbol.OID != "" {
		errs = append(
			errs,
			fmt.Errorf(
				"metric tag OID and symbol.OID cannot be both declared: OID=%s, symbol.OID=%s",
				metricTag.OID,
				metricTag.Symbol.OID,
			),
		)
	}
	if metricTag.OID != "" && metricTag.Symbol.OID == "" {
		metricTag.Symbol.OID = metricTag.OID
		metricTag.OID = ""
	}
	if isRawIndexMetricTag(*metricTag) {
		symbol := SymbolConfig(metricTag.Symbol)
		if symbol.Name == "" && metricTag.Tag == "" {
			errs = append(errs, errors.New("raw index metric tag requires `tag` or `symbol.name`"))
		}
		errs = append(errs, compileSymbolPatterns(&symbol))
		metricTag.Symbol = SymbolConfigCompat(symbol)
	} else if metricTag.Symbol.OID != "" || metricTag.Symbol.Name != "" {
		symbol := SymbolConfig(metricTag.Symbol)
		errs = append(errs, validateEnrichSymbol(&symbol, metricTagSymbol))
		metricTag.Symbol = SymbolConfigCompat(symbol)
	}
	if metricTag.LookupSymbol.OID != "" || metricTag.LookupSymbol.Name != "" {
		symbol := SymbolConfig(metricTag.LookupSymbol)
		errs = append(errs, validateEnrichSymbol(&symbol, metricTagSymbol))
		metricTag.LookupSymbol = SymbolConfigCompat(symbol)
		if metricTag.Table == "" {
			errs = append(errs, errors.New("`lookup_symbol` requires `table`"))
		}
	}
	if metricTag.Match != "" {
		pattern, err := regexp.Compile(metricTag.Match)
		if err != nil {
			errs = append(errs, fmt.Errorf("cannot compile `match` (`%s`): %s", metricTag.Match, err.Error()))
		} else {
			metricTag.Pattern = pattern
		}
		if len(metricTag.Tags) == 0 {
			errs = append(
				errs,
				fmt.Errorf("`tags` mapping must be provided if `match` (`%s`) is defined", metricTag.Match),
			)
		}
	}
	errs = append(errs, validateMapping(metricTag.Mapping, metricTagSymbol))
	if metricTag.Mapping.HasItems() && metricTag.Tag == "" && metricTag.Symbol.Name == "" {
		errs = append(
			errs,
			fmt.Errorf("`tag` or `symbol.name` must be provided if `mapping` (`%v`) is defined", metricTag.Mapping),
		)
	}
	for _, transform := range metricTag.IndexTransform {
		if transform.DropRight != 0 && transform.End != 0 {
			errs = append(
				errs,
				fmt.Errorf("transform rule cannot define both end and drop_right. Invalid rule: %#v", transform),
			)
		}
		if transform.DropRight == 0 && transform.Start > transform.End && transform.End != 0 {
			errs = append(
				errs,
				fmt.Errorf("transform rule end should be greater than start. Invalid rule: %#v", transform),
			)
		}
	}

	return errors.Join(errs...)
}

func isRawIndexMetricTag(metricTag MetricTagConfig) bool {
	if metricTag.Table != "" || metricTag.Symbol.OID != "" {
		return false
	}

	if metricTag.Index != 0 {
		return metricTag.Symbol.Format != "" ||
			metricTag.Symbol.ExtractValue != "" ||
			metricTag.Symbol.MatchPattern != "" ||
			metricTag.Mapping.HasItems()
	}

	return len(metricTag.IndexTransform) > 0 ||
		metricTag.Symbol.Format != "" ||
		metricTag.Symbol.ExtractValue != "" ||
		metricTag.Symbol.MatchPattern != "" ||
		metricTag.Mapping.HasItems()
}

func validateScalarMetricTag(path string, tag *MetricTagConfig) error {
	var errs []error
	if tag.Table != "" {
		errs = append(
			errs,
			fmt.Errorf("scalar metric_tags do not support `table` lookups (tag=%q, table=%q)", tag.Tag, tag.Table),
		)
	}
	if tag.Index != 0 {
		errs = append(
			errs,
			fmt.Errorf("scalar metric_tags do not support `index` lookups (tag=%q, index=%d)", tag.Tag, tag.Index),
		)
	}
	if len(tag.IndexTransform) > 0 {
		errs = append(errs, fmt.Errorf("scalar metric_tags do not support `index_transform` (tag=%q)", tag.Tag))
	}
	if tag.Symbol.OID == "" {
		errs = append(errs, fmt.Errorf("scalar metric_tags require `symbol.OID` (tag=%q)", tag.Tag))
	}
	if path != "" {
		for i, err := range errs {
			errs[i] = fmt.Errorf("%s: %w", path, err)
		}
	}
	return errors.Join(errs...)
}
