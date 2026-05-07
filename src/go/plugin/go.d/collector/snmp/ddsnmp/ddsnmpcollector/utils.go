// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

var errNoTextDateValue = errors.New("text_date: no timestamp value")

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
		parts = append(parts, fmt.Sprintf("%02x", v))
	}
	return strings.Join(parts, ":"), nil
}

func convPduToStringf(pdu gosnmp.SnmpPDU, format string) (string, error) {
	switch format {
	case "mac_address":
		return convPhysAddressToString(pdu)
	case "ip_address":
		return convPduToIPAddress(pdu)
	case "uint32":
		value, err := convNumericPduToInt64f(pdu, format)
		if err != nil {
			return "", err
		}
		return strconv.FormatInt(value, 10), nil
	case "hex":
		// Convert any value to hex string
		bs, ok := pdu.Value.([]byte)
		if !ok {
			return "", fmt.Errorf("cannot convert %T to hex", pdu.Value)
		}
		return hex.EncodeToString(bs), nil
	case "snmp_dateandtime":
		ts, err := convPduToDateAndTimeUnix(pdu)
		if err != nil {
			return "", err
		}
		return strconv.FormatInt(ts, 10), nil
	case "text_date":
		// Decode textual dates into unix timestamps directly from the
		// fresh PDU value on every poll.
		raw, err := convPduToString(pdu)
		if err != nil {
			return "", err
		}
		ts, ok := ddsnmp.ParseTextDate(raw)
		if !ok {
			if ddsnmp.IsTextDateNoValue(raw) {
				return "", errNoTextDateValue
			}
			return "", fmt.Errorf("text_date: cannot parse %q", raw)
		}
		return strconv.FormatInt(ts, 10), nil
	default:
		// For unknown formats, use the default string conversion
		return convPduToString(pdu)
	}
}

func convNumericPduToInt64f(pdu gosnmp.SnmpPDU, format string) (int64, error) {
	if !isPduNumericType(pdu) {
		return 0, fmt.Errorf("cannot convert %T to numeric value", pdu.Value)
	}

	value := gosnmp.ToBigInt(pdu.Value).Int64()

	switch format {
	case "uint32":
		if value < 0 {
			return int64(uint32(value)), nil
		}
	}

	return value, nil
}

func convPduToDateAndTimeUnix(pdu gosnmp.SnmpPDU) (int64, error) {
	var bs []byte

	switch v := pdu.Value.(type) {
	case []byte:
		bs = v
	case string:
		bs = []byte(v)
	default:
		return 0, fmt.Errorf("cannot convert %T to SNMP DateAndTime", pdu.Value)
	}

	if len(bs) != 8 && len(bs) != 11 {
		return 0, fmt.Errorf("invalid SNMP DateAndTime length %d", len(bs))
	}

	year := int(bs[0])<<8 | int(bs[1])
	month := time.Month(bs[2])
	day := int(bs[3])
	hour := int(bs[4])
	minute := int(bs[5])
	second := int(bs[6])
	deci := int(bs[7])

	// The 8-octet SNMPv2-TC DateAndTime form omits timezone fields when
	// only local time is known. The collector does not know the device's
	// timezone, so it uses UTC as a deterministic fallback; 11-octet values
	// use their embedded UTC offset below.
	loc := time.UTC
	if len(bs) == 11 {
		sign := bs[8]
		tzHours := int(bs[9])
		tzMinutes := int(bs[10])
		if sign != '+' && sign != '-' {
			return 0, fmt.Errorf("invalid SNMP DateAndTime UTC direction %q", sign)
		}
		if tzHours > 13 {
			return 0, fmt.Errorf("invalid SNMP DateAndTime UTC hours offset %d", tzHours)
		}
		if tzMinutes > 59 {
			return 0, fmt.Errorf("invalid SNMP DateAndTime UTC minutes offset %d", tzMinutes)
		}
		offset := tzHours*3600 + tzMinutes*60
		if sign == '-' {
			offset = -offset
		}
		loc = time.FixedZone("snmp", offset)
	}

	if second == 60 {
		return 0, fmt.Errorf("unsupported SNMP DateAndTime leap second")
	}

	tm := time.Date(year, month, day, hour, minute, second, deci*100_000_000, loc)
	if tm.Year() != year || tm.Month() != month || tm.Day() != day || tm.Hour() != hour || tm.Minute() != minute || tm.Second() != second || tm.Nanosecond()/100_000_000 != deci {
		return 0, fmt.Errorf("invalid SNMP DateAndTime value")
	}
	return tm.Unix(), nil
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
