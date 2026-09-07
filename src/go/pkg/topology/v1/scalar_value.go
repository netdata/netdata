// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

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
		if _, ok := integerValue(value); !ok {
			return "is not an integer"
		}
	case "uint":
		if n, ok := integerValue(value); !ok || n < 0 {
			return "is not a non-negative integer"
		}
	case "float", "duration":
		if _, ok := numberValue(value); !ok {
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
