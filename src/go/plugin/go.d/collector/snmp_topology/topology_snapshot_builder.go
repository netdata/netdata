// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func buildLocalTopologyDevice(dev ddsnmp.DeviceConnectionInfo) topologymodel.Device {
	device := topologymodel.Device{
		ManagementIP:       normalizeEligibleManagementIP(dev.Hostname),
		ChartIDPrefix:      topologyProfileChartIDPrefix,
		ChartContextPrefix: topologyProfileChartContextPrefix,
		SysObjectID:        dev.SysObjectID,
		SysName:            dev.SysName,
		SysDescr:           dev.SysDescr,
		SysContact:         dev.SysContact,
		SysLocation:        dev.SysLocation,
		Vendor:             dev.Vendor,
		Model:              dev.Model,
	}

	if dev.VnodeGUID != "" {
		device.AgentID = dev.VnodeGUID
		device.NetdataHostID = dev.VnodeGUID
	}

	if len(dev.VnodeLabels) > 0 {
		device.Labels = cloneTopologyLabels(dev.VnodeLabels)
	}
	device.Vendor, device.Model = ddsnmp.ResolveDeviceIdentity(device.Vendor, device.Model, nil, device.Labels)
	metadata := newTopologyMetadataIndex(device.Labels)

	if value := metadata.value(topologyMetadataAliasSysDescr); value != "" && device.SysDescr == "" {
		device.SysDescr = value
	}
	if value := metadata.value(topologyMetadataAliasSysContact); value != "" && device.SysContact == "" {
		device.SysContact = value
	}
	if value := metadata.value(topologyMetadataAliasSysLocation); value != "" && device.SysLocation == "" {
		device.SysLocation = value
	}
	if value := metadata.value(topologyMetadataAliasVendor); value != "" && device.Vendor == "" {
		device.Vendor = value
	}
	if value := metadata.value(topologyMetadataAliasModel); value != "" && device.Model == "" {
		device.Model = value
	}
	if value := topologyutil.NormalizeTopologyRouterID(device.Labels[tagOSPFRouterID]); value != "" {
		device.OSPFRouterID = value
		setTopologyMetadataLabelIfMissing(device.Labels, tagOSPFRouterID, value)
	}

	if value := metadata.value(topologyMetadataAliasSysUptime); value != "" {
		if uptime := topologyutil.ParsePositiveInt64(value); uptime > 0 {
			device.SysUptime = uptime
		}
	}
	if value := metadata.value(topologyMetadataAliasSerial); value != "" {
		device.SerialNumber = value
		setTopologyMetadataLabelIfMissing(device.Labels, "serial_number", value)
	}
	if value := metadata.value(topologyMetadataAliasSoftware); value != "" {
		device.SoftwareVersion = value
		setTopologyMetadataLabelIfMissing(device.Labels, "software_version", value)
	}
	if value := metadata.value(topologyMetadataAliasFirmware); value != "" {
		device.FirmwareVersion = value
		setTopologyMetadataLabelIfMissing(device.Labels, "firmware_version", value)
	}
	if value := metadata.value(topologyMetadataAliasHardware); value != "" {
		device.HardwareVersion = value
		setTopologyMetadataLabelIfMissing(device.Labels, "hardware_version", value)
	}

	return device
}
