// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"math"
	"strconv"
	"strings"
	"time"
)

func topologyCanonicalPortName(attrs map[string]any) string {
	if name := topologyAttrString(attrs, "port_name"); name != "" {
		return name
	}
	if name := topologyAttrString(attrs, "if_name"); name != "" {
		return name
	}
	if name := topologyAttrString(attrs, "if_descr"); name != "" {
		return name
	}
	if name := topologyAttrString(attrs, "if_alias"); name != "" {
		return name
	}

	if ifIndex := topologyAttrInt(attrs, "if_index"); ifIndex > 0 {
		return strconv.Itoa(ifIndex)
	}

	if portID := topologyAttrString(attrs, "port_id"); portID != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(portID)); err == nil && n > 0 {
			return strconv.Itoa(n)
		}
		return portID
	}
	if bridgePort := topologyAttrString(attrs, "bridge_port"); bridgePort != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(bridgePort)); err == nil && n > 0 {
			return strconv.Itoa(n)
		}
		return bridgePort
	}
	return ""
}

func topologyCanonicalLinkName(srcName, srcPortName, dstName, dstPortName string) string {
	srcName = strings.TrimSpace(srcName)
	if srcName == "" {
		srcName = "[unset]"
	}
	dstName = strings.TrimSpace(dstName)
	if dstName == "" {
		dstName = "[unset]"
	}
	srcPortName = strings.TrimSpace(srcPortName)
	if srcPortName == "" {
		srcPortName = "[unset]"
	}
	dstPortName = strings.TrimSpace(dstPortName)
	if dstPortName == "" {
		dstPortName = "[unset]"
	}
	return srcName + ":" + srcPortName + " -> " + dstName + ":" + dstPortName
}

func topologyAttrString(attrs map[string]any, key string) string {
	if len(attrs) == 0 {
		return ""
	}
	value, ok := attrs[key]
	if !ok || value == nil {
		return ""
	}
	str, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(str)
}

func topologyAttrInt(attrs map[string]any, key string) int {
	if len(attrs) == 0 {
		return 0
	}
	value, ok := attrs[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		if typed < 0 {
			return 0
		}
		if typed > math.MaxInt {
			return math.MaxInt
		}
		return int(typed)
	case float64:
		if typed <= 0 {
			return 0
		}
		if typed > math.MaxInt {
			return math.MaxInt
		}
		return int(typed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil || parsed <= 0 {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func topologyAttrStringSlice(attrs map[string]any, key string) []string {
	if len(attrs) == 0 {
		return nil
	}
	value, ok := attrs[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			str, ok := item.(string)
			if !ok {
				continue
			}
			if str = strings.TrimSpace(str); str != "" {
				out = append(out, str)
			}
		}
		return out
	default:
		return nil
	}
}

func topologyTimePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	out := t
	return &out
}
