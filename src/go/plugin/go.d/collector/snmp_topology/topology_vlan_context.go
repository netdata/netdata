// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
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

	profiles := loadTopologyVLANContextProfiles(dev)
	if recorder.evidence != nil {
		if len(profiles) == 0 {
			recorder.evidence.vlanProfiles = topologyAcquisitionPhaseEvidence{outcome: topologyAcquisitionPhaseEmpty}
		} else {
			recorder.evidence.vlanProfiles = successfulAcquisitionPhase()
		}
	}

	for i, context := range contexts {
		if ctx.Err() != nil {
			return
		}
		contextOrdinal := uint32(i + 1)
		observer := recorder.beginContext(contextOrdinal, context.vlanID, context.vlanName)

		pms, failure, err := collectTopologyVLANContext(ctx, c, dev, context.vlanID, profiles, observer)
		if err != nil {
			if captured := recorder.contextByOrdinal(contextOrdinal); captured != nil {
				switch failure {
				case topologyAcquisitionFailureClientConfiguration:
					captured.client = failedAcquisitionPhase(failure)
				case topologyAcquisitionFailureConnect:
					captured.client = successfulAcquisitionPhase()
					captured.connect = failedAcquisitionPhase(failure)
				case topologyAcquisitionFailureCollection:
					captured.client = successfulAcquisitionPhase()
					captured.connect = successfulAcquisitionPhase()
				}
			}
			recorder.completeContext(contextOrdinal, failedAcquisitionPhase(failure))
			if ctx.Err() != nil {
				return
			}
			c.Warningf("device '%s': topology vlan-context polling failed for vlan %s: %v", dev.Hostname, context.vlanID, err)
			continue
		}
		if captured := recorder.contextByOrdinal(contextOrdinal); captured != nil {
			captured.client = successfulAcquisitionPhase()
			captured.connect = successfulAcquisitionPhase()
		}
		recorder.completeContext(contextOrdinal, successfulAcquisitionPhase())
		applyTopologySemanticEvent(cache, topologySemanticEvent{
			kind:     topologySemanticEventVLANContext,
			profiles: pms,
			vlanID:   context.vlanID,
			vlanName: context.vlanName,
		})
	}
}
