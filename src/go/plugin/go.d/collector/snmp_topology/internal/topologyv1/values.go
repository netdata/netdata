// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strings"
	"time"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	topologyapi "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

var topologyV1IDInvalidChars = regexp.MustCompile(`[^A-Za-z0-9_.:-]+`)

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

func topologyV1EndpointString(endpoint topologymodel.LinkEndpoint, key string) string {
	switch key {
	case "if_name":
		return strings.TrimSpace(endpoint.IfName)
	case "if_descr":
		return strings.TrimSpace(endpoint.IfDescr)
	case "if_alias":
		return strings.TrimSpace(endpoint.IfAlias)
	case "port_id":
		return strings.TrimSpace(endpoint.PortID)
	case "port_name":
		return strings.TrimSpace(endpoint.PortName)
	case "management_ip":
		return strings.TrimSpace(endpoint.ManagementIP)
	case "sys_name":
		return topologyutil.FirstNonEmptyString(endpoint.SysName, topologyV1MatchString(endpoint.Match, key))
	default:
		return topologyV1MatchString(endpoint.Match, key)
	}
}

func nullableEndpointIfIndex(endpoint topologymodel.LinkEndpoint) any {
	if endpoint.IfIndex <= 0 {
		return nil
	}
	return uint64(endpoint.IfIndex)
}

func topologyV1EndpointPortName(endpoint topologymodel.LinkEndpoint) string {
	return topologyutil.FirstNonEmptyString(
		topologyV1EndpointString(endpoint, "port_name"),
		topologyV1EndpointString(endpoint, "if_name"),
		topologyV1EndpointString(endpoint, "if_descr"),
		topologyV1EndpointString(endpoint, "port_id"),
	)
}

func topologyV1MatchString(match topologymodel.Match, key string) string {
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

func nullableStringRef(dict *topologyapi.StringDictionary, value string) any {
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

func pruneNilAttributes(attrs map[string]any) map[string]any {
	for k, v := range attrs {
		switch vv := v.(type) {
		case string:
			if vv == "" {
				delete(attrs, k)
			}
		case []string:
			if len(vv) == 0 {
				delete(attrs, k)
			}
		case []map[string]any:
			if len(vv) == 0 {
				delete(attrs, k)
			}
		case nil:
			delete(attrs, k)
		default:
			rv := reflect.ValueOf(v)
			if rv.Kind() == reflect.Slice && rv.Len() == 0 {
				delete(attrs, k)
			}
		}
	}
	if len(attrs) == 0 {
		return nil
	}
	return attrs
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
