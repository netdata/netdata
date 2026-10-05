// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"encoding/json"
	"math"
	"strconv"
)

// scalarColumnValueError preserves the item type when set aggregation produces
// an array. A negative index identifies the cell itself rather than a member.
func scalarColumnValueError(typ, aggregation string, nullable bool, value any) (int, string) {
	if aggregation == "set" {
		if members, ok := value.([]any); ok {
			if members == nil {
				// A typed nil slice encodes as null, not as an empty JSON array.
				return -1, scalarItemError(typ, nullable, nil)
			}
			for i, member := range members {
				if reason := scalarItemError(typ, nullable, member); reason != "" {
					return i, reason
				}
			}
			return -1, ""
		}
	}
	return -1, scalarItemError(typ, nullable, value)
}

func scalarItemError(typ string, nullable bool, value any) string {
	if value == nil {
		if nullable {
			return ""
		}
		return "is null but column is not nullable"
	}
	switch typ {
	case "bool":
		if _, ok := value.(bool); !ok {
			return "is not a bool"
		}
	case "int":
		if _, ok := scalarNumberValue(value, true); !ok {
			return "is not an integer"
		}
	case "uint":
		if n, ok := scalarNumberValue(value, true); !ok || n < 0 {
			return "is not a non-negative integer"
		}
	case "float", "duration":
		if _, ok := scalarNumberValue(value, false); !ok {
			return "is not a number"
		}
	case "string", "ip", "mac", "timestamp":
		if _, ok := value.(string); !ok {
			return "is not a string"
		}
	default:
		return "has unsupported scalar column type"
	}
	return ""
}

// scalarNumberValue validates cells independently of machine-sized indexes.
// The returned number is only for sign checks; the stored value is unchanged.
func scalarNumberValue(raw any, integral bool) (float64, bool) {
	var n float64
	switch value := raw.(type) {
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case uint64:
		return float64(value), true
	case float64:
		n = value
	case json.Number:
		if !json.Valid([]byte(value)) {
			return 0, false
		}
		if integral {
			// Integer-form literals retain exact parsing, including the unsigned range.
			if len(value) > 0 && value[0] == '-' {
				integer, err := value.Int64()
				return float64(integer), err == nil
			}
			integer, err := strconv.ParseUint(string(value), 10, 64)
			return float64(integer), err == nil
		}
		var err error
		n, err = value.Float64()
		if err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	return n, !math.IsNaN(n) && !math.IsInf(n, 0) && (!integral || math.Trunc(n) == n)
}
