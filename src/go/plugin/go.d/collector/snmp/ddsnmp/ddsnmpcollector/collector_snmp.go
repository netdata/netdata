// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"

	"github.com/netdata/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

// requestConfig holds the configuration values a request needs: the SNMP
// version and the maximum OIDs per Get the client was configured with.
type requestConfig struct {
	version gosnmp.Version
	maxOIDs int
}

// Count attempted batches here so callers cannot lose or duplicate failed work.
func getSNMPValues(
	ctx context.Context,
	client snmputils.ScalarClient,
	req requestConfig,
	oids []string,
	missingOIDs map[string]bool,
	stats *ddsnmp.CollectionStats,
) (map[string]gosnmp.VarBind, error) {
	pdus := make(map[string]gosnmp.VarBind)
	for chunk := range slices.Chunk(oids, req.maxOIDs) {
		for len(chunk) > 0 {
			stats.SNMP.GetRequests++
			stats.SNMP.GetOIDs += int64(len(chunk))
			result, err := client.Get(ctx, chunk)
			var status *gosnmp.StatusError
			if errors.As(err, &status) {
				if missing, ok := v1MissingOID(req, status, chunk); ok {
					// V1 fails the whole request for one absent OID. Record that operation's
					// missing value before retrying the remaining OIDs on an owned slice.
					// Request diagnostics retain NoSuchName evidence even if this recovery succeeds.
					recordMissingOID(client, gosnmp.VarBind{Name: chunk[missing], Value: gosnmp.NoSuchObjectValue()}, missingOIDs, stats)
					chunk = slices.Delete(slices.Clone(chunk), missing, missing+1)
					continue
				}
				err = snmputils.WithPacketFailure(fmt.Errorf("SNMP GET response error %s", status.Status), "get", status)
			}
			if err == nil && result == nil {
				err = fmt.Errorf("SNMP GET returned nil response")
			}
			if err != nil {
				stats.Errors.SNMP++
				return nil, err
			}
			for _, pdu := range result.VarBinds {
				if !isPduWithData(pdu) {
					recordMissingOID(client, pdu, missingOIDs, stats)
					continue
				}
				pdus[trimOID(pdu.Name)] = pdu
			}
			break
		}
	}
	return pdus, nil
}

// v1MissingOID returns the position in chunk of the OID an SNMPv1 noSuchName
// answer names. The error-index reads 255 for any larger index, so with more
// than 255 OIDs an index of 255 names no OID for certain.
func v1MissingOID(req requestConfig, status *gosnmp.StatusError, chunk []string) (int, bool) {
	if req.version != gosnmp.Version1 || status.Status != gosnmp.NoSuchName {
		return 0, false
	}
	index := int(status.Index)
	if index < 1 || index > len(chunk) || index == 255 && len(chunk) > 255 {
		return 0, false
	}
	return index - 1, true
}

func recordMissingOID(client snmputils.ScalarClient, pdu gosnmp.VarBind, missingOIDs map[string]bool, stats *ddsnmp.CollectionStats) {
	stats.Errors.MissingOIDs++
	missingOIDs[trimOID(pdu.Name)] = true
	if observer, ok := client.(interface{ recordMissing(gosnmp.VarBind) }); ok {
		observer.recordMissing(pdu)
	}
}
