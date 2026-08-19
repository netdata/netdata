// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// ParseMetricFloat converts a supported profile scalar to a finite float.
func ParseMetricFloat(value any) (float64, bool) {
	f, ok := rawMetricFloat(value)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

func rawMetricFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// ParseMetricStateTTL parses a positive profile metric state TTL.
func ParseMetricStateTTL(value string) (time.Duration, error) {
	ttl, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	if ttl <= 0 {
		return 0, fmt.Errorf("must be greater than zero")
	}
	return ttl, nil
}
