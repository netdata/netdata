// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

const (
	prioFamilyPeerStates = collectorapi.Priority + iota
	prioFamilyPrefixes
	prioFamilyCorrectness
	prioFamilyMessages
	prioFamilyRIBRoutes
	prioFamilyPeerInventory
	prioRPKICacheState
	prioRPKICacheUptime
	prioRPKICacheRecords
	prioRPKICachePrefixes
	prioRPKIInventoryPrefixes
	prioVNIEntries
	prioVNIRemoteVTEPs
	prioPeerState
	prioPeerUptime
	prioPeerPrefixes
	prioPeerPolicyPrefixes
	prioPeerAdvertisedPrefixes
	prioNeighborTransitions
	prioNeighborChurn
	prioNeighborLastResetState
	prioNeighborLastResetAge
	prioNeighborLastErrorCodes
	prioPeerMessages
	prioNeighborMessageTypes
	prioCollectorStatus
	prioCollectorScrapeDuration
	prioCollectorFailures
	prioCollectorDeepQueries
)
