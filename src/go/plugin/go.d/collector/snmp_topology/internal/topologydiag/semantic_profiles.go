// SPDX-License-Identifier: GPL-3.0-or-later

package topologydiag

import (
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
)

var errSemanticBGPCollection = errors.New("captured BGP collection failure")

func validateAcquisitionEvidence(evidence *AcquisitionAttemptEvidence) error {
	if evidence == nil {
		return errors.New("acquisition evidence is missing")
	}
	if evidence.CollectedAt.IsZero() {
		return errors.New("acquisition collected time is missing")
	}
	if evidence.FreshFor <= 0 {
		return errors.New("acquisition freshness duration must be positive")
	}
	main := acquisitionContextByOrdinal(evidence, 0)
	if main == nil || main.Collection.Outcome != AcquisitionPhaseSuccess {
		return errors.New("successful main acquisition context is missing")
	}
	var previousContext uint32
	for i, context := range evidence.CollectionContexts {
		if i > 0 && context.Ordinal <= previousContext {
			return fmt.Errorf("acquisition context order: ordinal %d follows %d", context.Ordinal, previousContext)
		}
		previousContext = context.Ordinal
		var previousProfile uint32
		for j, profile := range context.Profiles {
			if j > 0 && profile.Identity.Ordinal <= previousProfile {
				return fmt.Errorf("acquisition profile order: ordinal %d follows %d", profile.Identity.Ordinal, previousProfile)
			}
			previousProfile = profile.Identity.Ordinal
		}
	}
	return nil
}

func acquisitionContextByOrdinal(
	evidence *AcquisitionAttemptEvidence,
	ordinal uint32,
) *AcquisitionContextEvidence {
	if evidence == nil {
		return nil
	}
	for i := range evidence.CollectionContexts {
		if evidence.CollectionContexts[i].Ordinal == ordinal {
			return &evidence.CollectionContexts[i]
		}
	}
	return nil
}

func SemanticProfiles(
	profiles []AcquisitionProfileEvidence,
) []*ddsnmp.ProfileMetrics {
	result := make([]*ddsnmp.ProfileMetrics, 0, len(profiles))
	for _, profile := range profiles {
		if profile.Outcome == ddsnmpcollector.AcquisitionProfileOutcomeFailed ||
			profile.Outcome == ddsnmpcollector.AcquisitionProfileOutcomeUnknown {
			continue
		}
		result = append(result, semanticProfileFromAcquisition(profile.Values))
	}
	return result
}

func semanticProfileFromAcquisition(profile AcquisitionProfileValues) *ddsnmp.ProfileMetrics {
	// The semantic builder only reads these immutable evidence maps; borrowing
	// them avoids rebuilding the acquisition value store during replay.
	result := &ddsnmp.ProfileMetrics{
		DeviceMetadata: profile.Metadata,
		Tags:           profile.Tags,
	}
	for _, metric := range profile.TopologyMetrics {
		result.TopologyMetrics = append(result.TopologyMetrics, ddsnmp.Metric{
			TopologyKind: metric.Kind,
			Tags:         metric.Tags,
		})
	}
	for _, row := range profile.BGPRows {
		result.BGPRows = append(result.BGPRows, semanticBGPRowFromAcquisition(row))
	}
	if profile.BGPFailed {
		result.BGPCollectError = errSemanticBGPCollection
	}
	return result
}

func semanticBGPRowFromAcquisition(row AcquisitionBGPRowValue) ddsnmp.BGPRow {
	return ddsnmp.BGPRow{
		OriginProfileID: row.OriginProfileID,
		Table:           row.Table,
		RowKey:          row.RowKey,
		StructuralID:    row.StructuralID,
		Kind:            row.Kind,
		Identity: ddsnmp.BGPIdentity{
			RoutingInstance: row.RoutingInstance,
			Neighbor:        row.Neighbor,
			RemoteAS:        row.RemoteAS,
		},
		Descriptors: ddsnmp.BGPDescriptors{
			LocalAddress:    row.LocalAddress,
			LocalAS:         row.LocalAS,
			LocalIdentifier: row.LocalIdentifier,
			PeerIdentifier:  row.PeerIdentifier,
			PeerType:        row.PeerType,
			BGPVersion:      row.BGPVersion,
			Description:     row.Description,
		},
		Admin: ddsnmp.BGPAdmin{Enabled: ddsnmp.BGPBool{
			Has:   row.AdminHas,
			Value: row.AdminEnabled,
		}},
		State: ddsnmp.BGPState{
			Has:   row.StateHas,
			State: row.State,
			Raw:   row.StateRaw,
		},
		Connection: ddsnmp.BGPConnection{
			EstablishedUptime: ddsnmp.BGPInt64{
				Has:   row.EstablishedHas,
				Value: row.Established,
			},
			LastReceivedUpdateAge: ddsnmp.BGPInt64{
				Has:   row.UpdateAgeHas,
				Value: row.UpdateAge,
			},
		},
		Tags: row.Tags,
	}
}
