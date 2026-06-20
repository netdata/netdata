// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
)

var topologyV1IDInvalidChars = regexp.MustCompile(`[^A-Za-z0-9_.:-]+`)

func anyStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func topologyV1ScalarLabelValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return strings.TrimSpace(fmt.Sprint(typed))
	default:
		return ""
	}
}

func nullableUintValue(value any) any {
	out, ok := uintValue(value)
	if !ok {
		return nil
	}
	return out
}

func nullableOptionalUintValue(value topologyengine.OptionalValue[int]) any {
	if !value.Has || value.Value < 0 {
		return nil
	}
	return uint64(value.Value)
}

func nullableOptionalUint64Value(value topologyengine.OptionalValue[int64]) any {
	if !value.Has || value.Value < 0 {
		return nil
	}
	return uint64(value.Value)
}

func uintValue(value any) (uint64, bool) {
	switch typed := value.(type) {
	case int:
		if typed >= 0 {
			return uint64(typed), true
		}
	case int8:
		if typed >= 0 {
			return uint64(typed), true
		}
	case int16:
		if typed >= 0 {
			return uint64(typed), true
		}
	case int32:
		if typed >= 0 {
			return uint64(typed), true
		}
	case int64:
		if typed >= 0 {
			return uint64(typed), true
		}
	case uint:
		return uint64(typed), true
	case uint8:
		return uint64(typed), true
	case uint16:
		return uint64(typed), true
	case uint32:
		return uint64(typed), true
	case uint64:
		return typed, true
	case float32:
		if typed >= 0 && math.Trunc(float64(typed)) == float64(typed) {
			return uint64(typed), true
		}
	case float64:
		if typed >= 0 && math.Trunc(typed) == typed {
			return uint64(typed), true
		}
	}
	return 0, false
}

func topologyV1EndpointString(endpoint topologyLinkEndpoint, key string) string {
	return firstNonEmptyString(
		anyStringValue(endpoint.Attributes[key]),
		topologyV1MatchString(endpoint.Match, key),
	)
}

func topologyV1EndpointPortName(endpoint topologyLinkEndpoint) string {
	return firstNonEmptyString(
		topologyV1EndpointString(endpoint, "port_name"),
		topologyV1EndpointString(endpoint, "if_name"),
		topologyV1EndpointString(endpoint, "if_descr"),
		topologyV1EndpointString(endpoint, "port_id"),
	)
}

func topologyV1MatchString(match topologyMatch, key string) string {
	switch key {
	case "sys_name":
		return match.SysName
	case "sys_object_id":
		return match.SysObjectID
	default:
		return ""
	}
}

func firstString(values []string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func nullableTime(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func nullableStringRef(dict *topologyv1.StringDictionary, value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return dict.Ref(value)
}

func nullableJSON(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case map[string]any:
		if len(typed) == 0 {
			return nil
		}
	case map[string]string:
		if len(typed) == 0 {
			return nil
		}
	case []any:
		if len(typed) == 0 {
			return nil
		}
	case []map[string]any:
		if len(typed) == 0 {
			return nil
		}
	}
	return value
}

func stringArrayCell(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func isEmptyArrayCell(value any) bool {
	values, ok := value.([]any)
	return ok && len(values) == 0
}

func anyStringSlice(value any) []string {
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s := topologyV1ScalarLabelValue(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		rv := reflect.ValueOf(value)
		if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
			return nil
		}
		out := make([]string, 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			if s := topologyV1ScalarLabelValue(rv.Index(i).Interface()); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
}

func anyMapSlice(value any) ([]map[string]any, bool) {
	switch typed := value.(type) {
	case []map[string]any:
		return typed, true
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			row, ok := item.(map[string]any)
			if !ok {
				return nil, false
			}
			out = append(out, row)
		}
		return out, true
	default:
		return nil, false
	}
}

func sortedMapKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func topologyID(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	value = topologyV1IDInvalidChars.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_.:-")
	if value == "" {
		value = fallback
	}
	first := value[0]
	if (first >= 'A' && first <= 'Z') || (first >= 'a' && first <= 'z') {
		return value
	}
	return "x_" + value
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
