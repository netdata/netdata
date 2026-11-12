// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

// CollectDeviceMetadata acquires only device metadata, without creating metric,
// table, topology, or diagnostic state. Missing-OID suppression lasts one attempt.
// The client was configured with version and maxOIDs (0 meaning gosnmp.MaxOids).
func CollectDeviceMetadata(
	client snmputils.ScalarClient,
	version gosnmp.SnmpVersion,
	maxOIDs int,
	profiles []*ddsnmp.Profile,
	sysObjectID string,
	log *logger.Logger,
) (map[string]ddsnmp.MetaTag, error) {
	req := requestConfig{version: version, maxOIDs: snmputils.MaxOIDsOrDefault(maxOIDs)}
	collector := newDeviceMetadataCollector(client, req, make(map[string]bool), log, sysObjectID)
	collector.strict = true
	result := make(map[string]ddsnmp.MetaTag)
	for _, profile := range profiles {
		meta, err := collector.collect(profile)
		if err != nil {
			return nil, err
		}
		for key, value := range meta {
			ddsnmp.MergeMetaTag(result, key, value)
		}
	}
	return result, nil
}
