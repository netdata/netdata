// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// decodeIndexComponent validates the complete SMI index, including unselected components.
func decodeIndexComponent(index string, cfg *ddprofiledefinition.MetricIndexDecode) (string, error) {
	if cfg.Component == 0 || cfg.Component > uint(len(cfg.Components)) {
		return "", fmt.Errorf("index_decode component out of range")
	}
	parts := strings.Split(index, ".")
	pos, selected := 0, ""
	for i, component := range cfg.Components {
		width, bits := uint64(1), 32
		switch component.Type {
		case "integer":
		case "octet_string":
			bits = 8
			if component.Length != nil {
				width = uint64(*component.Length)
			} else {
				if pos >= len(parts) {
					return "", fmt.Errorf("index_decode component %d has no length", i+1)
				}
				length, err := parseIndexArc(parts[pos], 32)
				if err != nil {
					return "", fmt.Errorf("index_decode component %d has invalid length", i+1)
				}
				width = length
				pos++
			}
		default:
			return "", fmt.Errorf("index_decode component %d has unknown type %q", i+1, component.Type)
		}
		if width > uint64(len(parts)-pos) {
			return "", fmt.Errorf("index_decode component %d is truncated", i+1)
		}
		end := pos + int(width)
		for _, arc := range parts[pos:end] {
			if _, err := parseIndexArc(arc, bits); err != nil {
				return "", fmt.Errorf("index_decode component %d has invalid arc", i+1)
			}
		}
		if uint(i+1) == cfg.Component {
			selected = strings.Join(parts[pos:end], ".")
		}
		pos = end
	}
	if pos != len(parts) {
		return "", fmt.Errorf("index_decode has trailing arcs")
	}
	return selected, nil
}

func formatIndexOctetString(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	parts := strings.Split(raw, ".")
	value := make([]byte, len(parts))
	for i, arc := range parts {
		n, err := parseIndexArc(arc, 8)
		if err != nil {
			return "", fmt.Errorf("cannot convert index octet to text")
		}
		value[i] = byte(n)
	}
	return string(value), nil
}

func parseIndexArc(arc string, bits int) (uint64, error) {
	if arc == "" || arc[0] < '0' || arc[0] > '9' {
		return 0, fmt.Errorf("invalid unsigned index arc")
	}
	return strconv.ParseUint(arc, 10, bits)
}
