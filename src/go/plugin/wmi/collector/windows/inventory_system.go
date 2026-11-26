// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"math"
	"regexp"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

const (
	queryOSInfo          = "SELECT CSName, Caption, Version, BuildNumber, ProductType FROM Win32_OperatingSystem"
	querySystemInfo      = "SELECT Manufacturer, Model, NumberOfLogicalProcessors FROM Win32_ComputerSystem"
	queryProcessorInfo   = "SELECT Architecture, Name, CurrentClockSpeed FROM Win32_Processor WHERE DeviceID = 'CPU0'"
	queryInstalledMemory = "SELECT Capacity FROM Win32_PhysicalMemory"
)

// The caption is localized. Keep it intact and split out only recognizable Windows product names.
var windowsCaption = regexp.MustCompile(`(?i)^(?:Microsoft )?(Windows(?: Server)?) ([0-9]+(?:\.[0-9]+)*)(?: (.+))?$`)

// updateOSInfo sets the hostname to the Windows computer name, or to the
// endpoint when the reported name is not a usable hostname, and the OS labels.
func updateOSInfo(node *vnodes.VirtualNode, row map[string]any, endpoint string) {
	name := textProperty(row, "csname")
	if validHost(name) && netdataapi.SanitizeWireValue(name) == name {
		node.Hostname = name
	} else {
		node.Hostname = endpoint
	}

	caption := textProperty(row, "caption")
	setLabel(node.Labels, "_os_version", caption)
	var family, marketing, edition string
	switch readUnsigned(row["producttype"]).value {
	case 1:
		family = "Windows"
	case 2, 3:
		family = "Windows Server"
	}
	if parts := windowsCaption.FindStringSubmatch(caption); parts != nil && strings.EqualFold(parts[1], family) {
		marketing, edition = parts[2], parts[3]
	}
	setLabel(node.Labels, "_os_name", family)
	setLabel(node.Labels, "_os_marketing_version", marketing)
	setLabel(node.Labels, "_os_edition", edition)
	var kernel string
	if version, build := textProperty(row, "version"), textProperty(row, "buildnumber"); version != "" && build != "" {
		kernel = "Windows " + version + " Build: " + build
	}
	setLabel(node.Labels, "_kernel_version", kernel)
}

func updateSystemInfo(labels map[string]string, row map[string]any) {
	model, vendor := textProperty(row, "model"), textProperty(row, "manufacturer")
	setLabel(labels, "_hw_product_name", model)
	setLabel(labels, "_hw_sys_vendor", vendor)
	setLabel(labels, "_system_cores", positiveDecimal(readUnsigned(row["numberoflogicalprocessors"]).value))

	platform := virtualizationPlatform(model)
	if platform == "" {
		platform = virtualizationPlatform(vendor)
	}
	setLabel(labels, "_virtualization", platform)
	if platform != "" {
		labels["_hw_product_type"] = "vm"
		labels["_virt_detection"] = "wmi"
	} else {
		delete(labels, "_hw_product_type")
		delete(labels, "_virt_detection")
	}
}

// virtualizationPlatform matches the native Windows Agent's WMI model and manufacturer probe.
func virtualizationPlatform(value string) string {
	value = strings.ToLower(value)
	for _, known := range [...]struct{ match, platform string }{
		{"vmware", "vmware"},
		{"virtualbox", "oracle"},
		{"innotek", "oracle"},
		{"oracle corp", "oracle"},
		{"parallels", "parallels"},
		{"qemu", "qemu"},
		{"kvm", "kvm"},
		{"xen", "xen"},
		{"domu", "xen"},
		{"amazon", "amazon"},
		{"digitalocean", "digitalocean"},
		{"hyper-v", "microsoft"},
		{"microsoft hv", "microsoft"},
		{"virtual machine", "microsoft"},
	} {
		if strings.Contains(value, known.match) {
			return known.platform
		}
	}
	return ""
}

func updateProcessorInfo(labels map[string]string, row map[string]any) {
	setLabel(labels, "_system_cpu_model", textProperty(row, "name"))
	var architecture string
	if arch := readUnsigned(row["architecture"]); arch.valid {
		switch arch.value {
		case 0:
			architecture = "x86"
		case 5:
			architecture = "ARM"
		case 9:
			architecture = "x86_64"
		case 12:
			architecture = "ARM64"
		}
	}
	setLabel(labels, "_architecture", architecture)
	mhz := readUnsigned(row["currentclockspeed"]).value
	if mhz > math.MaxUint32 {
		mhz = 0
	}
	setLabel(labels, "_system_cpu_freq", positiveDecimal(mhz*1_000_000))
}

// installedMemory sums the memory module capacities in bytes. An unknown or
// zero capacity makes the total unknown rather than understated.
func installedMemory(rows []map[string]any) string {
	var total uint64
	for _, row := range rows {
		capacity := readUnsigned(row["capacity"]).value
		if capacity == 0 || capacity > math.MaxUint64-total {
			return ""
		}
		total += capacity
	}
	return positiveDecimal(total)
}
