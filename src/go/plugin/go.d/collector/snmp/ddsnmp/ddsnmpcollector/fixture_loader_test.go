// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
)

type snmpFixture struct {
	entries []gosnmp.SnmpPDU
	byOID   map[string]gosnmp.SnmpPDU
}

func mustLoadSNMPFixture(t *testing.T, path string) *snmpFixture {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open fixture %s: %v", path, err)
	}
	defer f.Close()

	fixture := &snmpFixture{byOID: make(map[string]gosnmp.SnmpPDU)}
	scanner := bufio.NewScanner(f)
	for lineNum := 1; scanner.Scan(); lineNum++ {
		pdu, ok, err := parseSNMPFixtureLine(scanner.Text())
		if err != nil {
			t.Fatalf("parse fixture %s:%d: %v", path, lineNum, err)
		}
		if !ok {
			continue
		}

		oid := trimOID(pdu.Name)
		pdu.Name = oid
		fixture.entries = append(fixture.entries, pdu)
		fixture.byOID[oid] = pdu
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan fixture %s: %v", path, err)
	}

	return fixture
}

func parseSNMPFixtureLine(line string) (gosnmp.SnmpPDU, bool, error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return gosnmp.SnmpPDU{}, false, nil
	}

	switch {
	case strings.Count(line, "|") >= 2:
		return parseSNMPRecLine(line)
	case strings.Contains(line, " = "):
		return parseSNMPWalkLine(line)
	default:
		return gosnmp.SnmpPDU{}, false, nil
	}
}

func parseSNMPRecLine(line string) (gosnmp.SnmpPDU, bool, error) {
	parts := strings.SplitN(line, "|", 3)
	if len(parts) != 3 {
		return gosnmp.SnmpPDU{}, false, fmt.Errorf("invalid snmprec line")
	}

	oid := trimOID(parts[0])
	kind := strings.TrimSpace(parts[1])
	raw := strings.TrimSpace(parts[2])

	switch kind {
	case "2":
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return gosnmp.SnmpPDU{}, false, err
		}
		return gosnmp.SnmpPDU{Name: oid, Type: gosnmp.Integer, Value: int(v)}, true, nil
	case "4":
		return gosnmp.SnmpPDU{Name: oid, Type: gosnmp.OctetString, Value: []byte(raw)}, true, nil
	case "4x", "64x":
		b, err := hex.DecodeString(raw)
		if err != nil {
			return gosnmp.SnmpPDU{}, false, err
		}
		return gosnmp.SnmpPDU{Name: oid, Type: gosnmp.OctetString, Value: b}, true, nil
	case "6":
		return gosnmp.SnmpPDU{Name: oid, Type: gosnmp.ObjectIdentifier, Value: raw}, true, nil
	case "65":
		v, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			return gosnmp.SnmpPDU{}, false, err
		}
		return gosnmp.SnmpPDU{Name: oid, Type: gosnmp.Counter32, Value: uint(v)}, true, nil
	case "66":
		v, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			return gosnmp.SnmpPDU{}, false, err
		}
		return gosnmp.SnmpPDU{Name: oid, Type: gosnmp.Gauge32, Value: uint(v)}, true, nil
	case "67":
		v, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			return gosnmp.SnmpPDU{}, false, err
		}
		return gosnmp.SnmpPDU{Name: oid, Type: gosnmp.TimeTicks, Value: uint32(v)}, true, nil
	case "70":
		v, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return gosnmp.SnmpPDU{}, false, err
		}
		return gosnmp.SnmpPDU{Name: oid, Type: gosnmp.Counter64, Value: v}, true, nil
	default:
		return gosnmp.SnmpPDU{}, false, fmt.Errorf("unsupported snmprec type %q", kind)
	}
}

func parseSNMPWalkLine(line string) (gosnmp.SnmpPDU, bool, error) {
	oid, rest, ok := strings.Cut(line, " = ")
	if !ok {
		return gosnmp.SnmpPDU{}, false, fmt.Errorf("invalid snmpwalk line")
	}
	oid = trimOID(strings.TrimSpace(oid))
	rest = strings.TrimSpace(rest)

	if rest == `""` {
		return gosnmp.SnmpPDU{Name: oid, Type: gosnmp.OctetString, Value: []byte{}}, true, nil
	}

	kind, raw, ok := strings.Cut(rest, ": ")
	if !ok {
		return gosnmp.SnmpPDU{}, false, fmt.Errorf("unsupported snmpwalk payload %q", rest)
	}

	raw = strings.TrimSpace(raw)
	switch kind {
	case "STRING":
		return gosnmp.SnmpPDU{Name: oid, Type: gosnmp.OctetString, Value: []byte(stripQuotes(raw))}, true, nil
	case "Hex-STRING":
		b, err := hex.DecodeString(strings.ReplaceAll(raw, " ", ""))
		if err != nil {
			return gosnmp.SnmpPDU{}, false, err
		}
		return gosnmp.SnmpPDU{Name: oid, Type: gosnmp.OctetString, Value: b}, true, nil
	case "INTEGER":
		v, err := strconv.ParseInt(stripEnumSuffix(raw), 10, 64)
		if err != nil {
			return gosnmp.SnmpPDU{}, false, err
		}
		return gosnmp.SnmpPDU{Name: oid, Type: gosnmp.Integer, Value: int(v)}, true, nil
	case "Gauge32":
		v, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			return gosnmp.SnmpPDU{}, false, err
		}
		return gosnmp.SnmpPDU{Name: oid, Type: gosnmp.Gauge32, Value: uint(v)}, true, nil
	case "Counter32":
		v, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			return gosnmp.SnmpPDU{}, false, err
		}
		return gosnmp.SnmpPDU{Name: oid, Type: gosnmp.Counter32, Value: uint(v)}, true, nil
	case "Counter64":
		v, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return gosnmp.SnmpPDU{}, false, err
		}
		return gosnmp.SnmpPDU{Name: oid, Type: gosnmp.Counter64, Value: v}, true, nil
	case "Timeticks":
		v, err := strconv.ParseUint(stripTimeticksValue(raw), 10, 32)
		if err != nil {
			return gosnmp.SnmpPDU{}, false, err
		}
		return gosnmp.SnmpPDU{Name: oid, Type: gosnmp.TimeTicks, Value: uint32(v)}, true, nil
	case "OID":
		return gosnmp.SnmpPDU{Name: oid, Type: gosnmp.ObjectIdentifier, Value: trimOID(raw)}, true, nil
	default:
		return gosnmp.SnmpPDU{}, false, fmt.Errorf("unsupported snmpwalk type %q", kind)
	}
}

func stripQuotes(s string) string {
	if unquoted, err := strconv.Unquote(s); err == nil {
		return unquoted
	}
	return strings.Trim(s, `"`)
}

func stripEnumSuffix(s string) string {
	s = strings.TrimSpace(s)
	if _, err := strconv.ParseInt(s, 10, 64); err == nil {
		return s
	}

	if close := strings.LastIndex(s, ")"); close == len(s)-1 {
		if open := strings.LastIndex(s[:close], "("); open >= 0 {
			candidate := strings.TrimSpace(s[open+1 : close])
			if _, err := strconv.ParseInt(candidate, 10, 64); err == nil {
				return candidate
			}
		}
	}
	return s
}

func stripTimeticksValue(s string) string {
	if strings.HasPrefix(s, "(") {
		if i := strings.Index(s, ")"); i > 1 {
			return s[1:i]
		}
	}
	return stripEnumSuffix(s)
}

func expectSNMPWalkFromFixture(mockHandler *snmpmock.MockHandler, version gosnmp.SnmpVersion, fixture *snmpFixture, oid string) {
	root := trimOID(oid)
	prefix := root + "."
	pdus := make([]gosnmp.SnmpPDU, 0)
	for _, pdu := range fixture.entries {
		if pdu.Name == root || strings.HasPrefix(pdu.Name, prefix) {
			pdus = append(pdus, pdu)
		}
	}

	expectSNMPWalk(mockHandler, version, root, pdus)
}

func mustExpectSNMPGetFromFixture(t *testing.T, mockHandler *snmpmock.MockHandler, fixture *snmpFixture, oids []string) {
	t.Helper()

	pdus := make([]gosnmp.SnmpPDU, 0, len(oids))
	for _, oid := range oids {
		trimmed := trimOID(oid)
		pdu, ok := fixture.byOID[trimmed]
		if !ok {
			t.Fatalf("fixture is missing expected GET OID %s", trimmed)
		}
		pdus = append(pdus, pdu)
	}

	expectSNMPGet(mockHandler, oids, pdus)
}

func TestParseSNMPWalkLine_IntegerEnumValue(t *testing.T) {
	pdu, ok, err := parseSNMPWalkLine(".1.3.6.1.2.1.2.2.1.8.1 = INTEGER: up(1)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected fixture line to parse")
	}
	if pdu.Type != gosnmp.Integer {
		t.Fatalf("unexpected PDU type: %v", pdu.Type)
	}

	value, ok := pdu.Value.(int)
	if !ok {
		t.Fatalf("unexpected value type %T", pdu.Value)
	}
	if value != 1 {
		t.Fatalf("unexpected parsed value: got %d, want 1", value)
	}
}
