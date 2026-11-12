// SPDX-License-Identifier: GPL-3.0-or-later

package snmputils

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/gosnmp"
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

func GetSysUptime(ctx context.Context, client ScalarClient) (int64, error) {
	value, _, err := GetSysUptimeWithDiagnostics(ctx, client)
	return value, err
}

// GetSysUptimeWithDiagnostics observes the response without changing the uptime
// fallback or operational error behavior used by GetSysUptime.
func GetSysUptimeWithDiagnostics(ctx context.Context, client ScalarClient) (int64, Failure, error) {
	packet, err := client.Get(ctx, sysUptimeOIDs())
	failure := ClassifyGetFailure(packet, err)
	value, err := sysUptimeFromResponse(packet, err)
	if failure.Reason == "" {
		failure = ClassifyFailure(err)
	}
	return value, failure, err
}

func sysUptimeFromResponse(packet *gosnmp.Message, err error) (int64, error) {
	var status *gosnmp.StatusError
	if errors.As(err, &status) {
		// An answer with an error-status carries no uptime, which is not a
		// failure of the caller; the diagnostics keep the status.
		return 0, nil
	}
	if err != nil {
		return 0, WithFailure(err, "get", "")
	}
	if packet == nil || len(packet.VarBinds) == 0 {
		return 0, nil
	}

	pdusByOID := make(map[string]gosnmp.VarBind, len(packet.VarBinds))
	for _, pdu := range packet.VarBinds {
		pdusByOID[strings.TrimPrefix(pdu.Name, ".")] = pdu
	}

	var lastErr error
	for _, source := range sysUptimeSources {
		pdu, ok := pdusByOID[source.oid]
		if !ok || !isPduWithData(pdu) {
			continue
		}

		value, err := sysUptimePduValue(pdu)
		if err != nil {
			lastErr = WithFailure(fmt.Errorf("OID '%s': %w", source.oid, err), "sys_uptime", "processing")
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

func sysUptimePduValue(pdu gosnmp.VarBind) (int64, error) {
	if !isSysUptimeNumericPdu(pdu) {
		return 0, fmt.Errorf("cannot convert %v to numeric uptime", pdu.Value.Type())
	}
	if v, ok := pdu.Value.Int64(); ok {
		return v, nil
	}
	v, _ := pdu.Value.Uint64()
	return int64(v), nil
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

func isSysUptimeNumericPdu(pdu gosnmp.VarBind) bool {
	switch pdu.Value.Type() {
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
