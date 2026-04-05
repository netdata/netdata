// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

type segmentProjectionBuilder struct {
	attachments               []Attachment
	adjacencies               []Adjacency
	layer                     string
	source                    string
	collectedAt               time.Time
	deviceByID                map[string]Device
	ifaceByDeviceIndex        map[string]Interface
	ifIndexByDeviceName       map[string]int
	bridgeLinks               []bridgeBridgeLinkRecord
	reporterAliases           map[string][]string
	endpointMatchByID         map[string]topology.Match
	endpointLabelsByID        map[string]map[string]string
	actorIndex                map[string]struct{}
	probabilisticConnectivity bool
	strategyConfig            topologyInferenceStrategyConfig
	out                       projectedSegments
	segmentIDs                []string
	segmentMatchByID          map[string]topology.Match
	segmentByID               map[string]*bridgeDomainSegment
	deviceSegmentEdgeSeen     map[string]struct{}
	endpointSegmentEdgeSeen   map[string]struct{}
	endpointSegmentCandidates map[string][]string
	segmentPortKeys           map[string]map[string]struct{}
	segmentIfIndexes          map[string]map[string]struct{}
	segmentIfNames            map[string]map[string]struct{}
	rawFDBObservations        fdbReporterObservation
	rawFDBReporterHints       map[string]map[string][]bridgePortRef
	fdbObservations           fdbReporterObservation
	fdbOwners                 map[string]fdbEndpointOwner
	deviceIdentityByID        map[string]topologyIdentityKeySet
	reporterSegmentIndex      segmentReporterIndex
	aliasOwnerIDs             map[string]map[string]struct{}
	managedDeviceIDs          map[string]struct{}
	managedDeviceIDList       []string
	allowedEndpointBySegment  map[string]map[string]struct{}
	strictEndpointBySegment   map[string]map[string]struct{}
	probableEndpointBySegment map[string]map[string]struct{}
	probableAttachmentModes   map[string]map[string]string
	assignedEndpoints         map[string]struct{}
	baseCandidatesByEndpoint  map[string][]string
	probableCandidatesByEP    map[string][]string
	strictLinkedEndpoints     map[string]struct{}
}

func newSegmentProjectionBuilder(
	attachments []Attachment,
	adjacencies []Adjacency,
	layer string,
	source string,
	collectedAt time.Time,
	deviceByID map[string]Device,
	ifaceByDeviceIndex map[string]Interface,
	ifIndexByDeviceName map[string]int,
	bridgeLinks []bridgeBridgeLinkRecord,
	reporterAliases map[string][]string,
	endpointMatchByID map[string]topology.Match,
	endpointLabelsByID map[string]map[string]string,
	actorIndex map[string]struct{},
	probabilisticConnectivity bool,
	strategyConfig topologyInferenceStrategyConfig,
) *segmentProjectionBuilder {
	return &segmentProjectionBuilder{
		attachments:               attachments,
		adjacencies:               adjacencies,
		layer:                     layer,
		source:                    source,
		collectedAt:               collectedAt,
		deviceByID:                deviceByID,
		ifaceByDeviceIndex:        ifaceByDeviceIndex,
		ifIndexByDeviceName:       ifIndexByDeviceName,
		bridgeLinks:               bridgeLinks,
		reporterAliases:           reporterAliases,
		endpointMatchByID:         endpointMatchByID,
		endpointLabelsByID:        endpointLabelsByID,
		actorIndex:                actorIndex,
		probabilisticConnectivity: probabilisticConnectivity,
		strategyConfig:            strategyConfig,
		out: projectedSegments{
			actors: make([]topology.Actor, 0),
			links:  make([]topology.Link, 0),
		},
	}
}

func (b *segmentProjectionBuilder) build() projectedSegments {
	if len(b.attachments) == 0 && len(b.adjacencies) == 0 {
		return b.out
	}
	if !b.initializeSegments() {
		return b.out
	}
	endpointIDs := b.initializeEndpointCandidates()
	b.assignProbableEndpoints(endpointIDs)
	b.assignRemainingProbableEndpoints(endpointIDs)
	b.emitLinks()
	sortTopologyLinks(b.out.links)
	return b.out
}
