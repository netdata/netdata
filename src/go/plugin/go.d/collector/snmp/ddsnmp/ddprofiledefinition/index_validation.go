// SPDX-License-Identifier: GPL-3.0-or-later

package ddprofiledefinition

import (
	"errors"
	"fmt"
	"strconv"
)

func validateIndexTransforms(transforms []MetricIndexTransform) error {
	var errs []error
	for i, transform := range transforms {
		var ruleErrs []error
		if transform.DropRight != 0 && transform.End != 0 {
			ruleErrs = append(ruleErrs, errors.New("transform rule cannot define both end and drop_right"))
		}
		if transform.DropRight == 0 && transform.Start > transform.End && transform.End != 0 {
			ruleErrs = append(ruleErrs, errors.New("transform rule end should be greater than start"))
		}
		if len(transform.Mapping) > 0 {
			if transform.Start != transform.End || transform.DropRight != 0 {
				ruleErrs = append(ruleErrs, errors.New("mapping requires a single-arc transform (start == end) without drop_right"))
			}
			for key, value := range transform.Mapping {
				if !isCanonicalIndexArc(key) || !isCanonicalIndexArc(value) {
					ruleErrs = append(ruleErrs, fmt.Errorf("mapping requires canonical uint32 decimal keys and values: %q: %q", key, value))
				}
			}
		}
		errs = append(errs, withValidationPath(fmt.Sprintf("index_transform[%d]", i), errors.Join(ruleErrs...)))
	}
	return errors.Join(errs...)
}

func isCanonicalIndexArc(value string) bool {
	arc, err := strconv.ParseUint(value, 10, 32)
	return err == nil && strconv.FormatUint(arc, 10) == value
}

func validateIndexDecode(tag *MetricTagConfig) error {
	decode := tag.IndexDecode
	if decode == nil {
		return nil
	}
	var errs []error
	if tag.Index != 0 || len(tag.IndexTransform) > 0 || len(tag.IndexFrom) > 0 || tag.Table != "" || tag.Symbol.OID != "" || tag.OID != "" || tag.LookupSymbol.OID != "" || tag.LookupSymbol.Name != "" || tag.LookupScope != nil {
		errs = append(errs, errors.New("index_decode cannot be combined with index, index_transform, index_from, table, OID, lookup_symbol, or lookup_scope"))
	}
	if len(decode.Components) == 0 {
		errs = append(errs, errors.New("index_decode requires a complete nonempty components layout"))
	}
	if decode.Component == 0 || decode.Component > uint(len(decode.Components)) {
		errs = append(errs, errors.New("index_decode component must select a one-based component in the layout"))
	}
	for i, component := range decode.Components {
		switch component.Type {
		case "integer":
			if component.Length != nil {
				errs = append(errs, fmt.Errorf("index_decode.components[%d]: integer does not support length", i))
			}
		case "octet_string":
			if component.Length != nil && *component.Length == 0 {
				errs = append(errs, fmt.Errorf("index_decode.components[%d]: fixed length must be positive", i))
			}
		default:
			errs = append(errs, fmt.Errorf("index_decode.components[%d]: unsupported type %q", i, component.Type))
		}
	}
	return errors.Join(errs...)
}

func validateLookupScope(scope *MetricLookupScope, table string, lookup SymbolConfigCompat) error {
	if scope == nil {
		return nil
	}
	var errs []error
	if table == "" || lookup.OID == "" {
		errs = append(errs, errors.New("lookup_scope requires table and lookup_symbol.OID"))
	}
	if len(scope.SourceIndexTransform) == 0 || len(scope.TargetIndexTransform) == 0 {
		errs = append(errs, errors.New("lookup_scope requires nonempty source_index_transform and target_index_transform"))
	}
	errs = append(errs, withValidationPath("lookup_scope.source_index_transform", validateIndexTransforms(scope.SourceIndexTransform)))
	errs = append(errs, withValidationPath("lookup_scope.target_index_transform", validateIndexTransforms(scope.TargetIndexTransform)))
	return errors.Join(errs...)
}
