// SPDX-License-Identifier: GPL-3.0-or-later

package engine

type deviceInterfaceCollector struct {
	ifIndexes    map[string]struct{}
	ifNames      map[string]struct{}
	ifTypes      map[string]int
	adminCounts  map[string]int
	operCounts   map[string]int
	portStatuses []topologyDevicePortStatus
	portEvidence map[int]*topologyDevicePortEvidence
}

type deviceInterfaceSummaryBuilder struct {
	interfaces          []Interface
	attachments         []Attachment
	adjacencies         []Adjacency
	deviceByID          map[string]Device
	ifIndexByDeviceName map[string]int
	bridgeLinks         []bridgeBridgeLinkRecord
	reporterAliases     map[string][]string
	collectors          map[string]*deviceInterfaceCollector
	managedAliasOwners  map[string]map[string]struct{}
}

func buildTopologyDeviceInterfaceSummaries(
	interfaces []Interface,
	attachments []Attachment,
	adjacencies []Adjacency,
	deviceByID map[string]Device,
	ifIndexByDeviceName map[string]int,
	bridgeLinks []bridgeBridgeLinkRecord,
	reporterAliases map[string][]string,
) map[string]topologyDeviceInterfaceSummary {
	return newDeviceInterfaceSummaryBuilder(
		interfaces,
		attachments,
		adjacencies,
		deviceByID,
		ifIndexByDeviceName,
		bridgeLinks,
		reporterAliases,
	).build()
}

func newDeviceInterfaceSummaryBuilder(
	interfaces []Interface,
	attachments []Attachment,
	adjacencies []Adjacency,
	deviceByID map[string]Device,
	ifIndexByDeviceName map[string]int,
	bridgeLinks []bridgeBridgeLinkRecord,
	reporterAliases map[string][]string,
) *deviceInterfaceSummaryBuilder {
	return &deviceInterfaceSummaryBuilder{
		interfaces:          interfaces,
		attachments:         attachments,
		adjacencies:         adjacencies,
		deviceByID:          deviceByID,
		ifIndexByDeviceName: ifIndexByDeviceName,
		bridgeLinks:         bridgeLinks,
		reporterAliases:     reporterAliases,
		collectors:          make(map[string]*deviceInterfaceCollector),
	}
}

func (b *deviceInterfaceSummaryBuilder) build() map[string]topologyDeviceInterfaceSummary {
	if len(b.interfaces) == 0 {
		return nil
	}

	b.collectInterfaces()
	if len(b.collectors) == 0 {
		return nil
	}

	b.managedAliasOwners = buildFDBAliasOwnerMap(b.reporterAliases)
	b.collectFDBAttachments()
	b.collectAdjacencyEvidence()
	b.collectBridgeLinkEvidence()

	return b.buildSummaries()
}
