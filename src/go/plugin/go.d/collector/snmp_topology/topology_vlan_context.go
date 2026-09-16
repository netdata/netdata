// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologydiag"
)

func (c *Collector) collectTopologyVTPVLANContexts(
	ctx context.Context,
	cache *topologyBuilder,
	dev ddsnmp.DeviceConnectionInfo,
	recorder *topologyAcquisitionRecorder,
) {
	if cache == nil {
		return
	}
	if ctx.Err() != nil {
		return
	}

	contexts := cache.vtpVLANContexts()
	if len(contexts) == 0 {
		return
	}

	view := resolveTopologyVLANProfileView(dev)
	profiles := view.Profiles()
	if recorder.evidence != nil {
		recorder.evidence.VLANProfileContext = view.Context()
		if len(profiles) == 0 {
			recorder.evidence.VLANProfiles = topologydiag.AcquisitionPhaseEvidence{Outcome: topologydiag.AcquisitionPhaseEmpty}
		} else {
			recorder.evidence.VLANProfiles = successfulAcquisitionPhase()
		}
	}

	for i, context := range contexts {
		if ctx.Err() != nil {
			return
		}
		contextOrdinal := uint32(i + 1)
		observer := recorder.beginContext(contextOrdinal, context.vlanID, context.vlanName)

		pms, progress, err := collectTopologyVLANContext(ctx, c, dev, context.vlanID, profiles, observer)
		if captured := recorder.contextByOrdinal(contextOrdinal); captured != nil {
			captured.Sources = progress.sources
			captured.Client, captured.Connect = progress.client, progress.connect
			captured.Interruption, captured.Failures = progress.interruption, progress.failures
		}
		recorder.completeContext(contextOrdinal, progress.collection)
		if err != nil {
			if ctx.Err() != nil {
				recorder.recordInterruption(ctx.Err())
				return
			}
			c.Warningf("device '%s': topology vlan-context polling failed for vlan %s: %v", dev.Hostname, context.vlanID, err)
			continue
		}
		applyTopologySemanticEvent(cache, topologySemanticEvent{
			kind:     topologySemanticEventVLANContext,
			profiles: pms,
			vlanID:   context.vlanID,
			vlanName: context.vlanName,
		})
	}
}
