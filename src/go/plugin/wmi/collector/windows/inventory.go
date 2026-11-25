// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"context"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/wmi/collector/windows/internal/wmiquery"
)

const (
	inventoryInterval    = 5 * time.Minute
	queryOSInfo          = "SELECT CSName, Caption, Version, BuildNumber, ProductType FROM Win32_OperatingSystem"
	querySystemInfo      = "SELECT Manufacturer, Model, NumberOfLogicalProcessors FROM Win32_ComputerSystem"
	queryProcessorInfo   = "SELECT Architecture, Name, CurrentClockSpeed FROM Win32_Processor WHERE DeviceID = 'CPU0'"
	queryInstalledMemory = "SELECT Capacity FROM Win32_PhysicalMemory"
)

func (c *Collector) refreshInventory(ctx context.Context, client queryClient, fixedDrives []map[string]any, diskQueryOK bool) {
	c.nextInventory = time.Now().Add(inventoryInterval)
	deadline, _ := ctx.Deadline()
	// Leave time to close the WMI session and publish metrics.
	ctx, cancel := context.WithTimeout(ctx, min(5*time.Second, time.Until(deadline)/2))
	defer cancel()
	// Each provider owns its fields. Publish one snapshot and retain the previous
	// values of providers that failed or could not run within the poll deadline.
	node := c.vnode.Copy()
	for _, source := range []struct {
		name  string
		query string
		apply func([]map[string]any)
	}{
		{"operating system", queryOSInfo, func(rows []map[string]any) { updateOSInfo(node, singleton(rows)) }},
		{"computer system", querySystemInfo, func(rows []map[string]any) { updateSystemInfo(node.Labels, singleton(rows)) }},
		{"processor", queryProcessorInfo, func(rows []map[string]any) { updateProcessorInfo(node.Labels, singleton(rows)) }},
		{"installed memory", queryInstalledMemory, func(rows []map[string]any) { setLabel(node.Labels, "_system_ram_total", installedMemory(rows)) }},
	} {
		if ctx.Err() != nil {
			break
		}
		rows, err := client.Query(ctx, source.query)
		if err != nil {
			c.logInventoryError(source.name, err)
			continue
		}
		source.apply(rows)
	}
	if diskQueryOK && ctx.Err() == nil {
		capacity, err := diskInventory(ctx, client, fixedDrives)
		if err != nil {
			c.logInventoryError("disk", err)
		} else {
			setLabel(node.Labels, "_system_disk_space", capacity)
		}
	}
	if ctx.Err() == nil {
		name, ip, err := networkInventory(ctx, client)
		if err != nil {
			c.logInventoryError("default interface", err)
		} else {
			setLabel(node.Labels, "_net_default_iface", name)
			setLabel(node.Labels, "_net_default_iface_ip", ip)
			if ip != "" {
				node.Labels["_net_default_iface_detection"] = "wmi"
			} else {
				delete(node.Labels, "_net_default_iface_detection")
			}
		}
	}
	c.vnode = node
}

func (c *Collector) logInventoryError(source string, err error) {
	c.Limit("wmi:inventory:"+source, 1, inventoryInterval).Warning(safeTransportError(source+" WMI inventory query failed", err))
}

func singleton(rows []map[string]any) map[string]any {
	if len(rows) == 1 {
		return rows[0]
	}
	return nil
}

func textProperty(row map[string]any, key string) string {
	v, _ := row[key].(string)
	return strings.TrimSpace(v)
}

func setLabel(labels map[string]string, key, value string) {
	if value == "" {
		delete(labels, key)
	} else {
		labels[key] = value
	}
}

func positiveDecimal(n uint64) string {
	if n == 0 {
		return ""
	}
	return strconv.FormatUint(n, 10)
}

// Caption is localized. Keep it intact and split out only recognizable Windows product names.
var windowsCaption = regexp.MustCompile(`(?i)^(?:Microsoft )?(Windows(?: Server)?) ([0-9]+(?:\.[0-9]+)*)(?: (.+))?$`)

func updateOSInfo(node *vnodes.VirtualNode, row map[string]any) {
	name := textProperty(row, "csname")
	if wmiquery.ValidHost(name) && netdataapi.SanitizeWireValue(name) == name {
		node.Hostname = name
	} else {
		node.Hostname = canonicalEndpoint(node.Labels["address"])
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

	platform := windowsVirtualization(model)
	if platform == "" {
		platform = windowsVirtualization(vendor)
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

// Match the native Windows Agent's WMI model/manufacturer probe.
func windowsVirtualization(value string) string {
	value = strings.ToLower(value)
	for _, item := range [...]struct{ match, platform string }{
		{"vmware", "vmware"}, {"virtualbox", "oracle"}, {"innotek", "oracle"}, {"oracle corp", "oracle"},
		{"parallels", "parallels"}, {"qemu", "qemu"}, {"kvm", "kvm"}, {"xen", "xen"}, {"domu", "xen"},
		{"amazon", "amazon"}, {"digitalocean", "digitalocean"}, {"hyper-v", "microsoft"}, {"microsoft hv", "microsoft"}, {"virtual machine", "microsoft"},
	} {
		if strings.Contains(value, item.match) {
			return item.platform
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
