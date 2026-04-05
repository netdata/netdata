// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

var (
	topologyMetadataAliasSysDescr = []string{
		"description", "sys_descr", "sys_description",
	}
	topologyMetadataAliasSysContact = []string{
		"contact", "sys_contact",
	}
	topologyMetadataAliasSysLocation = []string{
		"location", "sys_location",
	}
	topologyMetadataAliasVendor = []string{
		"vendor", "manufacturer",
	}
	topologyMetadataAliasModel = []string{
		"model", "device_model",
	}
	topologyMetadataAliasSysUptime = []string{
		"sys_uptime", "sysuptime", "uptime",
	}
	topologyMetadataAliasSerial = []string{
		"serial_number", "serial", "serial_num", "serial_no", "serialnumber",
	}
	topologyMetadataAliasFirmware = []string{
		"firmware_version", "firmware", "firmware_rev", "firmware_revision",
	}
	topologyMetadataAliasSoftware = []string{
		"software_version", "software", "software_rev", "software_revision",
		"sw_version", "sw_rev", "version", "os_version",
	}
	topologyMetadataAliasHardware = []string{
		"hardware_version", "hardware", "hardware_rev", "hw_version", "hw_rev",
	}
)
