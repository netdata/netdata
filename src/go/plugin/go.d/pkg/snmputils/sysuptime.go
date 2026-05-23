// SPDX-License-Identifier: GPL-3.0-or-later

package snmputils

import (
	"fmt"
	"strings"

	"github.com/gosnmp/gosnmp"
)

const (
	OidSnmpEngineTime  = "1.3.6.1.6.3.10.2.1.3.0"
	OidHrSystemUptime  = "1.3.6.1.2.1.25.1.1.0"
	OidSysUpTime       = "1.3.6.1.2.1.1.3.0"
	sysUptimeTimeTicks = 0.01
)

type sysUptimeSource struct {
	oid   string
	scale float64
}

var sysUptimeSources = []sysUptimeSource{
	{oid: OidSnmpEngineTime},
	{oid: OidHrSystemUptime, scale: sysUptimeTimeTicks},
	{oid: OidSysUpTime, scale: sysUptimeTimeTicks},
}

func GetSysUptime(client gosnmp.Handler) (int64, error) {
	packet, err := client.Get(sysUptimeOIDs())
	if err != nil {
		return 0, err
	}
	if packet == nil || len(packet.Variables) == 0 {
		return 0, nil
	}

	pdusByOID := make(map[string]gosnmp.SnmpPDU, len(packet.Variables))
	for _, pdu := range packet.Variables {
		pdusByOID[strings.TrimPrefix(pdu.Name, ".")] = pdu
	}

	var lastErr error
	for _, source := range sysUptimeSources {
		pdu, ok := pdusByOID[source.oid]
		if !ok || !isSysUptimePduWithData(pdu) {
			continue
		}

		value, err := sysUptimePduValue(pdu)
		if err != nil {
			lastErr = fmt.Errorf("OID '%s': %w", source.oid, err)
			continue
		}
		if source.scale != 0 {
			value = int64(float64(value) * source.scale)
		}
		if value > 0 {
			return value, nil
		}
	}

	return 0, lastErr
}

func sysUptimeOIDs() []string {
	oids := make([]string, 0, len(sysUptimeSources))
	for _, source := range sysUptimeSources {
		oids = append(oids, source.oid)
	}
	return oids
}

func sysUptimePduValue(pdu gosnmp.SnmpPDU) (int64, error) {
	if !isSysUptimeNumericPdu(pdu) {
		return 0, fmt.Errorf("cannot convert %T to numeric uptime", pdu.Value)
	}
	return gosnmp.ToBigInt(pdu.Value).Int64(), nil
}

func isSysUptimePduWithData(pdu gosnmp.SnmpPDU) bool {
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

func isSysUptimeNumericPdu(pdu gosnmp.SnmpPDU) bool {
	switch pdu.Type {
	case gosnmp.Counter32,
		gosnmp.Counter64,
		gosnmp.Integer,
		gosnmp.Gauge32,
		gosnmp.Uinteger32,
		gosnmp.TimeTicks:
		return true
	default:
		return false
	}
}
