// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

func chargeRenderWork(data topologymodel.Data, limiter worklimit.Limiter) error {
	if limiter == nil {
		return nil
	}
	base, err := worklimit.Sum(uint64(len(data.Actors)), uint64(len(data.Links)))
	if err != nil {
		return err
	}
	if err := limiter.Charge(base); err != nil {
		return err
	}

	var nested uint64
	add := func(values ...uint64) error {
		for _, value := range values {
			var err error
			nested, err = worklimit.Sum(nested, value)
			if err != nil {
				return err
			}
		}
		return nil
	}
	for _, actor := range data.Actors {
		matchWork, err := matchRenderWork(actor.Match)
		if err != nil {
			return err
		}
		if err := add(
			matchWork, uint64(len(actor.Labels)),
			uint64(len(actor.Detail.SNMP.ManagementAddresses)), uint64(len(actor.Detail.SNMP.Capabilities)),
			uint64(len(actor.Detail.SNMP.CapabilitiesSupported)), uint64(len(actor.Detail.SNMP.CapabilitiesEnabled)),
			uint64(len(actor.Detail.SNMP.DeviceCharts)), uint64(len(actor.Detail.SNMP.InterfaceCharts)),
			uint64(len(actor.Detail.L2.Device.Ports)), uint64(len(actor.Detail.OSPF)), uint64(len(actor.Detail.BGP)),
		); err != nil {
			return err
		}
		for _, chart := range actor.Detail.SNMP.InterfaceCharts {
			if err := add(uint64(len(chart.AvailableMetrics))); err != nil {
				return err
			}
		}
		for _, port := range actor.Detail.L2.Device.Ports {
			if err := add(uint64(len(port.Neighbors)), uint64(len(port.VLANs))); err != nil {
				return err
			}
		}
	}
	for _, link := range data.Links {
		srcWork, err := matchRenderWork(link.Src.Match)
		if err != nil {
			return err
		}
		dstWork, err := matchRenderWork(link.Dst.Match)
		if err != nil {
			return err
		}
		if err := add(srcWork, dstWork); err != nil {
			return err
		}
		if link.Detail.L3SubnetMembership != nil {
			if err := add(uint64(len(link.Detail.L3SubnetMembership.Interfaces))); err != nil {
				return err
			}
		}
	}
	return limiter.Charge(nested)
}

func matchRenderWork(match graph.Match) (uint64, error) {
	return worklimit.Sum(
		uint64(len(match.ChassisIDs)), uint64(len(match.MacAddresses)), uint64(len(match.IPAddresses)),
		uint64(len(match.Hostnames)), uint64(len(match.DNSNames)), uint64(len(match.ContainerIDs)),
		uint64(len(match.PodNames)), uint64(len(match.NamespaceIDs)),
	)
}
