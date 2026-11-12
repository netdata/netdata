// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/netdata/gosnmp"
	"github.com/stretchr/testify/require"
)

func loadSnmprecPDUs(t *testing.T, relativePath string) []gosnmp.VarBind {
	t.Helper()

	path := filepath.Join("testdata", relativePath)
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var pdus []gosnmp.VarBind
	for lineNum, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 3)
		require.Lenf(t, parts, 3, "invalid snmprec line %d", lineNum+1)

		name := parts[0]
		typeCode := parts[1]
		value := parts[2]

		pdu, err := snmprecPDU(name, typeCode, value)
		require.NoErrorf(t, err, "invalid snmprec line %d", lineNum+1)
		pdus = append(pdus, pdu)
	}

	return pdus
}

func snmprecPDU(name, typeCode, raw string) (gosnmp.VarBind, error) {
	switch typeCode {
	case "2":
		n, err := strconv.ParseInt(raw, 10, 32)
		if err != nil {
			return gosnmp.VarBind{}, err
		}
		return createIntegerPDU(name, int(n)), nil
	case "4":
		return createStringPDU(name, raw), nil
	case "4x":
		decoded, err := hex.DecodeString(raw)
		if err != nil {
			return gosnmp.VarBind{}, err
		}
		return createPDU(name, gosnmp.OctetString, decoded), nil
	case "6":
		return createPDU(name, gosnmp.ObjectIdentifier, strings.TrimPrefix(raw, ".")), nil
	case "64":
		return createPDU(name, gosnmp.IPAddress, raw), nil
	case "64x":
		decoded, err := hex.DecodeString(raw)
		if err != nil {
			return gosnmp.VarBind{}, err
		}
		if len(decoded) != 4 && len(decoded) != 16 {
			return gosnmp.VarBind{}, fmt.Errorf("unsupported 64x length %d", len(decoded))
		}
		return createPDU(name, gosnmp.IPAddress, decoded), nil
	case "65":
		n, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			return gosnmp.VarBind{}, err
		}
		return createCounter32PDU(name, uint(n)), nil
	case "66":
		n, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			return gosnmp.VarBind{}, err
		}
		return createGauge32PDU(name, uint(n)), nil
	case "67":
		n, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			return gosnmp.VarBind{}, err
		}
		return createTimeTicksPDU(name, uint32(n)), nil
	case "70":
		n, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return gosnmp.VarBind{}, err
		}
		return createCounter64PDU(name, n), nil
	default:
		return gosnmp.VarBind{}, fmt.Errorf("unsupported snmprec type %q", typeCode)
	}
}
