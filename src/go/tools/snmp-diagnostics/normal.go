// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"

	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
)

func executeNormalOperation(operation string, archive openedArchive, options commandOptions) (any, error) {
	device := archive.normal
	switch operation {
	case "validate":
		return struct {
			Valid    bool              `json:"valid"`
			Kind     string            `json:"kind"`
			Producer snmpdiag.Producer `json:"producer"`
		}{true, snmpdiag.KindNormal, archive.producer}, nil
	case "summary":
		return struct {
			Producer          snmpdiag.Producer `json:"producer"`
			RegistrationID    uint64            `json:"registration_id"`
			RuntimeID         uint64            `json:"runtime_id"`
			Hostname          string            `json:"hostname"`
			LatestAttempt     uint64            `json:"latest_attempt"`
			LatestFailed      bool              `json:"latest_failed"`
			RetainedFailure   bool              `json:"retained_failure"`
			SourceOperations  int               `json:"source_operations"`
			Samples           int               `json:"samples"`
			BGPCachedRows     int               `json:"bgp_cached_rows"`
			LicenseCachedRows int               `json:"license_cached_rows"`
		}{archive.producer, device.RegistrationID, device.RuntimeID, device.Hostname, device.Latest.ID, device.Latest.Failed,
			device.LastFailure != nil || device.Latest.Failed, len(device.Sources), len(device.Latest.Samples), len(device.Latest.BGP.Entries), len(device.Latest.Licensing.Rows)}, nil
	case "inspect-device":
		if options.registrationID != device.RegistrationID {
			return nil, fmt.Errorf("normal archive contains device %d", device.RegistrationID)
		}
		return device, nil
	default:
		return nil, fmt.Errorf("%s requires topology evidence", operation)
	}
}
