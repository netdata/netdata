// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func processRawIndexTagValue(cfg ddprofiledefinition.MetricTagConfig, raw string) (string, error) {
	val := raw

	if cfg.Symbol.ExtractValueCompiled != nil {
		sm := cfg.Symbol.ExtractValueCompiled.FindStringSubmatch(val)
		if len(sm) < 2 {
			return "", fmt.Errorf("extract_value did not match transformed index '%s'", raw)
		}
		val = sm[1]
	}

	if cfg.Symbol.MatchPatternCompiled != nil {
		sm := cfg.Symbol.MatchPatternCompiled.FindStringSubmatch(val)
		if len(sm) == 0 {
			return "", fmt.Errorf("match_pattern '%s' did not match transformed index '%s'", cfg.Symbol.MatchPattern, val)
		}
		val = replaceSubmatches(cfg.Symbol.MatchValue, sm)
	}

	if cfg.Symbol.Format != "" {
		formatted, err := formatIndexTagValue(val, cfg.Symbol.Format)
		if err != nil {
			return "", err
		}
		val = formatted
	}

	if mapped, ok := cfg.Mapping[val]; ok {
		val = mapped
	}

	return val, nil
}

func formatIndexTagValue(raw string, format string) (string, error) {
	switch format {
	case "", "string":
		return raw, nil
	case "ip_address":
		return formatIndexIPAddress(raw)
	default:
		return raw, nil
	}
}

func formatIndexIPAddress(raw string) (string, error) {
	if strings.Contains(raw, ":") {
		return raw, nil
	}

	parts := strings.Split(raw, ".")
	switch len(parts) {
	case 4, 8, 16, 20:
	default:
		return "", fmt.Errorf("cannot convert transformed index '%s' to IP address", raw)
	}

	bytes := make([]byte, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || n > 255 {
			return "", fmt.Errorf("cannot convert transformed index '%s' to IP address", raw)
		}
		bytes = append(bytes, byte(n))
	}

	if len(bytes) == 4 {
		return renderIPv4Bytes(bytes), nil
	}

	switch len(bytes) {
	case 8:
		return renderIPv4Bytes(bytes[:4]) + "%" + renderZoneIndex(bytes[4:8]), nil
	case 16:
		return renderIPv6Bytes(bytes), nil
	case 20:
		return renderIPv6Bytes(bytes[:16]) + "%" + renderZoneIndex(bytes[16:20]), nil
	}

	return "", fmt.Errorf("cannot convert transformed index '%s' to IP address", raw)
}
