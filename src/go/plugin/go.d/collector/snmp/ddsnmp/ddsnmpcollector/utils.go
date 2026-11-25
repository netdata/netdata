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

	"github.com/netdata/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

var errNoTextDateValue = errors.New("text_date: no timestamp value")

func getMetricTypeFromPDUType(pdu gosnmp.VarBind) ddprofiledefinition.ProfileMetricType {
	switch pdu.Value.Type() {
	case gosnmp.Counter32, gosnmp.Counter64:
		// Counters are submitted as rates by default
		return ddprofiledefinition.ProfileMetricTypeRate
	case gosnmp.Gauge32, gosnmp.Integer, gosnmp.Uinteger32, gosnmp.OpaqueFloat, gosnmp.OpaqueDouble:
		// Numeric types representing current values are submitted as gauges
		return ddprofiledefinition.ProfileMetricTypeGauge
	case gosnmp.TimeTicks:
		// TimeTicks (hundredths of a second) are typically static or slow changing values
		return ddprofiledefinition.ProfileMetricTypeGauge
	case gosnmp.IPAddress, gosnmp.OctetString, gosnmp.ObjectIdentifier:
		// String-like types typically get converted to a numeric value to represent presence (1)
		// or some extraction of numeric info from the string
		return ddprofiledefinition.ProfileMetricTypeGauge
	default:
		return ddprofiledefinition.ProfileMetricTypeGauge
	}
}

func convPhysAddressToString(pdu gosnmp.VarBind) (string, error) {
	address, ok := pdu.Value.Bytes()
	if !ok {
		return "", fmt.Errorf("physAddress is not an OctetString but %v", pdu.Value.Type())
	}

	parts := make([]string, 0, len(address))
	for _, v := range address {
		parts = append(parts, fmt.Sprintf("%02x", v))
	}
	return strings.Join(parts, ":"), nil
}

func convPduToStringf(pdu gosnmp.VarBind, format string) (string, error) {
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
		bs, ok := pdu.Value.Bytes()
		if !ok {
			return "", fmt.Errorf("cannot convert %v to hex", pdu.Value.Type())
		}
		return hex.EncodeToString(bs), nil
	case "snmp_dateandtime":
		if isEmptyOctetStringPDU(pdu) {
			return "", errNoTextDateValue
		}
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

func isEmptyOctetStringPDU(pdu gosnmp.VarBind) bool {
	bs, ok := pdu.Value.Bytes()
	return ok && len(bs) == 0
}

func convNumericPduToInt64f(pdu gosnmp.VarBind, format string) (int64, error) {
	if !isPduNumericType(pdu) {
		return 0, fmt.Errorf("cannot convert %v to numeric value", pdu.Value.Type())
	}

	value := pduInt64(pdu.Value)

	switch format {
	case "uint32":
		if value < 0 {
			return int64(uint32(value)), nil
		}
	}

	return value, nil
}

// pduInt64 reads an integer value as int64: a Counter64 above 2^63-1 keeps its
// low 64 bits, and the Opaque floats, which the metric path reads itself
// (valueProcessor), read 0, as the untyped integer conversion did.
func pduInt64(v gosnmp.Value) int64 {
	if n, ok := v.Int64(); ok {
		return n
	}
	if n, ok := v.Uint64(); ok {
		return int64(n)
	}
	return 0
}

func convPduToDateAndTimeUnix(pdu gosnmp.VarBind) (int64, error) {
	bs, ok := pdu.Value.Bytes()
	if !ok {
		return 0, fmt.Errorf("cannot convert %v to SNMP DateAndTime", pdu.Value.Type())
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

func convPduToString(pdu gosnmp.VarBind) (string, error) {
	switch pdu.Value.Type() {
	case gosnmp.NoSuchObject, gosnmp.NoSuchInstance, gosnmp.Null:
		return "", fmt.Errorf("object not available: %v", pdu.Value.Type())
	case gosnmp.OctetString:
		bs, _ := pdu.Value.Bytes()
		if utf8.Valid(bs) {
			return strings.ToValidUTF8(string(bs), "�"), nil
		}
		return hex.EncodeToString(bs), nil
	case gosnmp.Counter32, gosnmp.Integer, gosnmp.Gauge32, gosnmp.Uinteger32, gosnmp.TimeTicks:
		v, _ := pdu.Value.Int64()
		return strconv.FormatInt(v, 10), nil
	case gosnmp.Counter64:
		v, _ := pdu.Value.Uint64()
		return strconv.FormatUint(v, 10), nil
	case gosnmp.IPAddress:
		ip, _ := pdu.Value.IP()
		if !ip.IsValid() {
			// An empty IpAddress, which some devices send.
			return "", errors.New("IPAddress has unexpected type <nil>")
		}
		return ip.String(), nil
	case gosnmp.ObjectIdentifier:
		v, _ := pdu.Value.OID()
		return strings.TrimPrefix(v, "."), nil
	default:
		return pdu.Value.String(), nil
	}
}

func isPduWithData(pdu gosnmp.VarBind) bool {
	switch pdu.Value.Type() {
	case gosnmp.NoSuchObject,
		gosnmp.NoSuchInstance,
		gosnmp.Null,
		gosnmp.EndOfMibView:
		return false
	default:
		return true
	}
}

func isPduNumericType(pdu gosnmp.VarBind) bool {
	switch pdu.Value.Type() {
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
