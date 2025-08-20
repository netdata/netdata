// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func getMetricTypeFromPDUType(pdu gosnmp.SnmpPDU) ddprofiledefinition.ProfileMetricType {
	switch pdu.Type {
	case gosnmp.Counter32, gosnmp.Counter64:
		// Counters are submitted as rates by default
		return ddprofiledefinition.ProfileMetricTypeRate
	case gosnmp.Gauge32, gosnmp.Integer, gosnmp.Uinteger32, gosnmp.OpaqueFloat, gosnmp.OpaqueDouble:
		// Numeric types representing current values are submitted as gauges
		return ddprofiledefinition.ProfileMetricTypeGauge
	case gosnmp.TimeTicks:
		// TimeTicks (hundredths of a second) are typically static or slow changing values
		return ddprofiledefinition.ProfileMetricTypeGauge
	case gosnmp.IPAddress, gosnmp.OctetString, gosnmp.ObjectIdentifier, gosnmp.BitString:
		// String-like types typically get converted to a numeric value to represent presence (1)
		// or some extraction of numeric info from the string
		return ddprofiledefinition.ProfileMetricTypeGauge
	case gosnmp.Boolean:
		// Boolean values (true/false) fit naturally as gauges (1/0)
		return ddprofiledefinition.ProfileMetricTypeGauge
	default:
		return ddprofiledefinition.ProfileMetricTypeGauge
	}
}

func convPhysAddressToString(pdu gosnmp.SnmpPDU) (string, error) {
	address, ok := pdu.Value.([]uint8)
	if !ok {
		return "", fmt.Errorf("physAddress is not a []uint8 or []byte but %T", pdu.Value)
	}

	parts := make([]string, 0, len(address))
	for _, v := range address {
		parts = append(parts, fmt.Sprintf("%02X", v))
	}
	return strings.Join(parts, ":"), nil
}

func convPduToStringf(pdu gosnmp.SnmpPDU, format string) (string, error) {
	switch format {
	case "mac_address":
		return convPhysAddressToString(pdu)
	case "ip_address":
		if pdu.Type == gosnmp.IPAddress {
			// Use the default handler for IP addresses
			return convPduToString(pdu)
		}

		// Try to handle as bytes that represent an IP
		bs, ok := pdu.Value.([]byte)
		if !ok {
			return "", fmt.Errorf("cannot convert %T to IP address", pdu.Value)
		}

		if len(bs) == 4 {
			// IPv4
			return fmt.Sprintf("%d.%d.%d.%d", bs[0], bs[1], bs[2], bs[3]), nil
		} else if len(bs) == 16 {
			// IPv6
			parts := make([]string, 0, 8)
			for i := 0; i < 16; i += 2 {
				parts = append(parts, fmt.Sprintf("%02x%02x", bs[i], bs[i+1]))
			}
			return strings.Join(parts, ":"), nil
		}

		return "", fmt.Errorf("cannot convert %v to IP address (incorrect length)", pdu.Value)
	case "hex":
		// Convert any value to hex string
		bs, ok := pdu.Value.([]byte)
		if !ok {
			return "", fmt.Errorf("cannot convert %T to hex", pdu.Value)
		}
		return hex.EncodeToString(bs), nil
	default:
		// For unknown formats, use the default string conversion
		return convPduToString(pdu)
	}
}

func convPduToString(pdu gosnmp.SnmpPDU) (string, error) {
	switch pdu.Type {
	case gosnmp.NoSuchObject, gosnmp.NoSuchInstance, gosnmp.Null:
		return "", fmt.Errorf("object not available: %v", pdu.Type)
	case gosnmp.OctetString:
		var bs []byte
		switch v := pdu.Value.(type) {
		case []byte:
			bs = v
		case string:
			return v, nil
		default:
			return "", fmt.Errorf("OctetString has unexpected type %T", pdu.Value)
		}

		if utf8.Valid(bs) {
			return strings.ToValidUTF8(string(bs), "�"), nil
		}
		return hex.EncodeToString(bs), nil
	case gosnmp.Counter32, gosnmp.Counter64, gosnmp.Integer, gosnmp.Gauge32, gosnmp.Uinteger32, gosnmp.TimeTicks:
		return gosnmp.ToBigInt(pdu.Value).String(), nil
	case gosnmp.IPAddress:
		switch v := pdu.Value.(type) {
		case []byte:
			if len(v) == 4 {
				return fmt.Sprintf("%d.%d.%d.%d", v[0], v[1], v[2], v[3]), nil
			} else if len(v) == 16 {
				parts := make([]string, 0, 8)
				for i := 0; i < 16; i += 2 {
					parts = append(parts, fmt.Sprintf("%02x%02x", v[i], v[i+1]))
				}
				return strings.Join(parts, ":"), nil
			}
			return hex.EncodeToString(v), nil
		case string:
			return v, nil
		default:
			return "", fmt.Errorf("IPAddress has unexpected type %T", pdu.Value)
		}
	case gosnmp.ObjectIdentifier:
		v, ok := pdu.Value.(string)
		if !ok {
			return "", fmt.Errorf("ObjectIdentifier is not a string but %T", pdu.Value)
		}
		return strings.TrimPrefix(v, "."), nil
	case gosnmp.Boolean:
		b, ok := pdu.Value.(bool)
		if !ok {
			return "", fmt.Errorf("boolean is not a bool but %T", pdu.Value)
		}
		if b {
			return "true", nil
		}
		return "false", nil
	default:
		return fmt.Sprintf("%v", pdu.Value), nil
	}
}

func isPduWithData(pdu gosnmp.SnmpPDU) bool {
	switch pdu.Type {
	case gosnmp.NoSuchObject,
		gosnmp.NoSuchInstance,
		gosnmp.Null,
		gosnmp.EndOfMibView:
		return false
	default:
		return true
	}
}

func isPduNumericType(pdu gosnmp.SnmpPDU) bool {
	switch pdu.Type {
	case gosnmp.Counter32,
		gosnmp.Counter64,
		gosnmp.Integer,
		gosnmp.Gauge32,
		gosnmp.Uinteger32,
		gosnmp.TimeTicks,
		gosnmp.OpaqueFloat,
		gosnmp.OpaqueDouble:
		return true
	default:
		return false
	}
}

func trimOID(oid string) string {
	return strings.TrimPrefix(oid, ".")
}

var reBackRef = regexp.MustCompile(`([$\\])(\d+)`)

func replaceSubmatches(template string, submatches []string) string {
	return reBackRef.ReplaceAllStringFunc(template, func(match string) string {
		parts := reBackRef.FindStringSubmatch(match)
		if len(parts) < 3 {
			return match
		}

		groupNum, err := strconv.Atoi(parts[2])
		if err != nil || groupNum >= len(submatches) {
			return match
		}

		return submatches[groupNum]
	})
}

func ternary[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}

func isInt(s string) bool {
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}

func mergeMetaTagIfAbsent(dest map[string]ddsnmp.MetaTag, key string, tag ddsnmp.MetaTag) {
	if existing, ok := dest[key]; !ok || existing.Value == "" || (!existing.IsExactMatch && tag.IsExactMatch) {
		dest[key] = tag
	}
}
