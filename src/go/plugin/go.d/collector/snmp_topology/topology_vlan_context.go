// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

type topologyVLANContextState uint8

const (
	topologyVLANContextUnknown topologyVLANContextState = iota
	topologyVLANContextSuccess
	topologyVLANContextFailed
	topologyVLANContextIncomplete
)

type topologyVLANContextResult struct {
	ordinal            uint32
	vlanID             string
	vlanName           string
	state              topologyVLANContextState
	profiles           []*ddsnmp.ProfileMetrics
	profileDefinitions []*ddsnmp.Profile
	reason             string
}

func (c *Collector) collectTopologyVTPVLANContexts(
	ctx context.Context,
	contexts []topologyVLANContext,
	dev ddsnmp.DeviceConnectionInfo,
	consumeSuccess func(topologyVLANContextResult),
	observeOutcome func(topologyVLANContextResult),
) {
	if ctx.Err() != nil || len(contexts) == 0 || (consumeSuccess == nil && observeOutcome == nil) {
		return
	}

	profiles, err := loadTopologyVLANContextProfiles(dev)
	if err != nil {
		c.Warningf("device '%s': topology vlan-context polling disabled: failed to load profiles: %v", dev.Hostname, err)
		for ordinal, context := range contexts {
			if observeOutcome == nil {
				break
			}
			observeOutcome(topologyVLANContextResult{
				ordinal: uint32(ordinal),
				vlanID:  context.vlanID, vlanName: context.vlanName,
				state: topologyVLANContextFailed, reason: "profile_load_failed",
			})
		}
		return
	}

	for ordinal, context := range contexts {
		if ctx.Err() != nil {
			if observeOutcome != nil {
				observeOutcome(topologyVLANContextResult{
					ordinal: uint32(ordinal),
					vlanID:  context.vlanID, vlanName: context.vlanName,
					state: topologyVLANContextIncomplete, reason: "cancelled",
				})
			}
			break
		}

		pms, err := collectTopologyVLANContext(ctx, c, dev, context.vlanID, profiles)
		if err != nil {
			if ctx.Err() != nil {
				if observeOutcome != nil {
					observeOutcome(topologyVLANContextResult{
						ordinal: uint32(ordinal),
						vlanID:  context.vlanID, vlanName: context.vlanName,
						state: topologyVLANContextIncomplete, reason: "cancelled",
					})
				}
				break
			}
			c.Warningf("device '%s': topology vlan-context polling failed for vlan %s: %v", dev.Hostname, context.vlanID, err)
			if observeOutcome != nil {
				observeOutcome(topologyVLANContextResult{
					ordinal: uint32(ordinal),
					vlanID:  context.vlanID, vlanName: context.vlanName,
					state: topologyVLANContextFailed, reason: topologyVLANFailureReason(err),
				})
			}
			continue
		}
		result := topologyVLANContextResult{
			ordinal: uint32(ordinal),
			vlanID:  context.vlanID, vlanName: context.vlanName,
			state: topologyVLANContextSuccess, profiles: pms, profileDefinitions: profiles,
		}
		if consumeSuccess != nil {
			consumeSuccess(result)
		}
		if observeOutcome != nil {
			observeOutcome(result)
		}
	}
}

func topologyVLANFailureReason(err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "cancelled"
	}
	return "collection_failed"
}
