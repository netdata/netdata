// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"time"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func buildLocalTopologyDevice(dev ddsnmp.DeviceConnectionInfo) topologyDevice {
	device := topologyDevice{
		ManagementIP:       dev.Hostname,
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

	if value := topologyMetadataValue(device.Labels, topologyMetadataAliasSysDescr); value != "" && device.SysDescr == "" {
		device.SysDescr = value
	}
	if value := topologyMetadataValue(device.Labels, topologyMetadataAliasSysContact); value != "" && device.SysContact == "" {
		device.SysContact = value
	}
	if value := topologyMetadataValue(device.Labels, topologyMetadataAliasSysLocation); value != "" && device.SysLocation == "" {
		device.SysLocation = value
	}
	if value := topologyMetadataValue(device.Labels, topologyMetadataAliasVendor); value != "" && device.Vendor == "" {
		device.Vendor = value
	}
	if value := topologyMetadataValue(device.Labels, topologyMetadataAliasModel); value != "" && device.Model == "" {
		device.Model = value
	}

	if value := topologyMetadataValue(device.Labels, topologyMetadataAliasSysUptime); value != "" {
		if uptime := parsePositiveInt64(value); uptime > 0 {
			device.SysUptime = uptime
		}
	}
	if value := topologyMetadataValue(device.Labels, topologyMetadataAliasSerial); value != "" {
		device.SerialNumber = value
		setTopologyMetadataLabelIfMissing(device.Labels, "serial_number", value)
	}
	if value := topologyMetadataValue(device.Labels, topologyMetadataAliasSoftware); value != "" {
		device.SoftwareVersion = value
		setTopologyMetadataLabelIfMissing(device.Labels, "software_version", value)
	}
	if value := topologyMetadataValue(device.Labels, topologyMetadataAliasFirmware); value != "" {
		device.FirmwareVersion = value
		setTopologyMetadataLabelIfMissing(device.Labels, "firmware_version", value)
	}
	if value := topologyMetadataValue(device.Labels, topologyMetadataAliasHardware); value != "" {
		device.HardwareVersion = value
		setTopologyMetadataLabelIfMissing(device.Labels, "hardware_version", value)
	}

	return device
}

func (c *topologyCache) snapshot() (topologyData, bool) {
	if !c.hasFreshSnapshotAt(time.Now()) {
		return topologyData{}, false
	}

	local := c.localDevice
	local = normalizeTopologyDevice(local)

	observations, localDeviceID := c.buildEngineObservations(local)
	if len(observations) == 0 {
		return topologyData{}, false
	}

	result, err := topologyengine.BuildL2ResultFromObservations(observations, topologyengine.DiscoverOptions{
		EnableLLDP:   true,
		EnableCDP:    true,
		EnableBridge: true,
		EnableARP:    true,
	})
	if err != nil {
		return topologyData{}, false
	}

	data := topologyengine.ToTopologyData(result, topologyengine.TopologyDataOptions{
		SchemaVersion:  topologySchemaVersion,
		Source:         "snmp",
		Layer:          "2",
		View:           "summary",
		AgentID:        c.agentID,
		LocalDeviceID:  localDeviceID,
		CollectedAt:    c.lastUpdate,
		ResolveDNSName: resolveTopologyReverseDNSName,
	})

	augmentLocalActorFromCache(&data, local)
	return data, true
}
