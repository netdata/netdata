// SPDX-License-Identifier: GPL-3.0-or-later

package model

type ResultStats struct {
	DevicesTotal                       int
	LinksTotal                         int
	LinksLLDP                          int
	LinksCDP                           int
	LinksSTP                           int
	AttachmentsTotal                   int
	AttachmentsFDB                     int
	EnrichmentsTotal                   int
	EnrichmentsARPND                   int
	BridgeDomainsTotal                 int
	EndpointsTotal                     int
	IdentityAliasEndpointsMapped       int
	IdentityAliasEndpointsAmbiguousMAC int
	IdentityAliasIPsMerged             int
	IdentityAliasIPsConflictSkipped    int
}

type ProjectionStats struct {
	ResultStats

	DevicesDiscovered          int
	LinksBidirectional         int
	LinksUnidirectional        int
	LinksFDB                   int
	LinksFDBEndpointCandidates int
	LinksFDBEndpointEmitted    int
	LinksFDBEndpointSuppressed int
	EndpointsAmbiguousSegments int
	LinksARP                   int
	LinksProbable              int
	SegmentsSuppressed         int
	ActorsTotal                int
	ActorsUnlinkedSuppressed   int
	InferenceStrategy          string
}
