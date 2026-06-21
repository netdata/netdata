// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import "strings"

const (
	maxProjectionInt   = int(^uint(0) >> 1)
	maxProjectionInt64 = int64(1<<63 - 1)
)

func topologyAttrsHaveAny(attrs map[string]any, keys ...string) bool {
	if len(attrs) == 0 {
		return false
	}
	for _, key := range keys {
		if _, ok := attrs[key]; ok {
			return true
		}
	}
	return false
}

func topologyOptionalAttrInt(attrs map[string]any, key string) OptionalValue[int] {
	if !topologyAttrsHaveAny(attrs, key) {
		return OptionalValue[int]{}
	}
	return OptionalValue[int]{Value: topologyAttrInt(attrs, key), Has: true}
}

func topologyOptionalAttrInt64(attrs map[string]any, key string) OptionalValue[int64] {
	if !topologyAttrsHaveAny(attrs, key) {
		return OptionalValue[int64]{}
	}
	return OptionalValue[int64]{Value: topologyAttrInt64(attrs, key), Has: true}
}

func topologyAttrInt64(attrs map[string]any, key string) int64 {
	if len(attrs) == 0 {
		return 0
	}
	value, ok := attrs[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int8:
		return int64(typed)
	case int16:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case uint:
		if uint64(typed) > uint64(maxProjectionInt64) {
			return maxProjectionInt64
		}
		return int64(typed)
	case uint8:
		return int64(typed)
	case uint16:
		return int64(typed)
	case uint32:
		return int64(typed)
	case uint64:
		if typed > uint64(maxProjectionInt64) {
			return maxProjectionInt64
		}
		return int64(typed)
	case float32:
		if float64(typed) > float64(maxProjectionInt64) {
			return maxProjectionInt64
		}
		return int64(typed)
	case float64:
		if typed > float64(maxProjectionInt64) {
			return maxProjectionInt64
		}
		return int64(typed)
	case string:
		return parseTopologyLabelInt64(typed)
	default:
		return 0
	}
}

func topologyAttrIntMap(attrs map[string]any, key string) map[string]int {
	if len(attrs) == 0 {
		return nil
	}
	value, ok := attrs[key]
	if !ok || value == nil {
		return nil
	}
	out := make(map[string]int)
	switch typed := value.(type) {
	case map[string]int:
		for k, v := range typed {
			if k = strings.TrimSpace(k); k != "" {
				out[k] = v
			}
		}
	case map[string]any:
		for k, v := range typed {
			if k = strings.TrimSpace(k); k != "" {
				out[k] = topologyInt64ToInt(topologyAnyInt64Value(v))
			}
		}
	default:
		return nil
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func topologyAnyInt64Value(value any) int64 {
	return topologyAttrInt64(map[string]any{"value": value}, "value")
}

func topologyInt64ToInt(value int64) int {
	if value < 0 {
		return 0
	}
	if value > int64(maxProjectionInt) {
		return maxProjectionInt
	}
	return int(value)
}
