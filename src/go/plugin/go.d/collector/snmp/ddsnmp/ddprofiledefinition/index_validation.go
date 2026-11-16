// SPDX-License-Identifier: GPL-3.0-or-later

package ddprofiledefinition

import (
	"errors"
	"fmt"
)

func validateIndexDecode(tag *MetricTagConfig) error {
	decode := tag.IndexDecode
	if decode == nil {
		return nil
	}
	var errs []error
	if tag.Index != 0 || len(tag.IndexTransform) > 0 || len(tag.IndexFrom) > 0 || tag.Table != "" || tag.Symbol.OID != "" || tag.OID != "" || tag.LookupSymbol.OID != "" || tag.LookupSymbol.Name != "" {
		errs = append(errs, errors.New("index_decode cannot be combined with index, index_transform, index_from, table, OID, or lookup_symbol"))
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
