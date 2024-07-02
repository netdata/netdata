// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
)

const (
	metricHypervHealthCritical = "windows_hyperv_health_critical"
	metricHypervHealthOK       = "windows_hyperv_health_ok"

	metricHypervRootPartition4KGPAPages                    = "windows_hyperv_root_partition_4K_gpa_pages"
	metricHypervRootPartition2MGPAPages                    = "windows_hyperv_root_partition_2M_gpa_pages"
	metricHypervRootPartition1GGPAPages                    = "windows_hyperv_root_partition_1G_gpa_pages"
	metricHypervRootPartition4KDevicePages                 = "windows_hyperv_root_partition_4K_device_pages"
	metricHypervRootPartition2MDevicePages                 = "windows_hyperv_root_partition_2M_device_pages"
	metricHypervRootPartition1GDevicePages                 = "windows_hyperv_root_partition_1G_device_pages"
	metricHypervRootPartitionGPASpaceModifications         = "windows_hyperv_root_partition_gpa_space_modifications"
	metricHypervRootPartitionAttachedDevices               = "windows_hyperv_root_partition_attached_devices"
	metricHypervRootPartitionDepositedPages                = "windows_hyperv_root_partition_deposited_pages"
	metricHypervRootPartitionPhysicalPagesAllocated        = "windows_hyperv_root_partition_physical_pages_allocated" // SkippedTimerTicks
	metricHypervRootPartitionDeviceDMAErrors               = "windows_hyperv_root_partition_device_dma_errors"
	metricHypervRootPartitionDeviceInterruptErrors         = "windows_hyperv_root_partition_device_interrupt_errors"
	metricHypervRootPartitionDeviceInterruptThrottleEvents = "windows_hyperv_root_partition_device_interrupt_throttle_events"
	metricHypervRootPartitionIOTLBFlush                    = "windows_hyperv_root_partition_io_tlb_flush"
	metricHypervRootPartitionAddressSpace                  = "windows_hyperv_root_partition_address_spaces"
	metricHypervRootPartitionVirtualTLBPages               = "windows_hyperv_root_partition_virtual_tlb_pages"
	metricHypervRootPartitionVirtualTLBFlushEntries        = "windows_hyperv_root_partition_virtual_tlb_flush_entires"

	metricsHypervVMCPUGuestRunTime      = "windows_hyperv_vm_cpu_guest_run_time"
	metricsHypervVMCPUHypervisorRunTime = "windows_hyperv_vm_cpu_hypervisor_run_time"
	metricsHypervVMCPURemoteRunTime     = "windows_hyperv_vm_cpu_remote_run_time"
	metricsHypervVMCPUTotalRunTime      = "windows_hyperv_vm_cpu_total_run_time"

	metricHypervVMMemoryPhysical             = "windows_hyperv_vm_memory_physical"
	metricHypervVMMemoryPhysicalGuestVisible = "windows_hyperv_vm_memory_physical_guest_visible"
	metricHypervVMMemoryPressureCurrent      = "windows_hyperv_vm_memory_pressure_current"
	metricHyperVVIDPhysicalPagesAllocated    = "windows_hyperv_vid_physical_pages_allocated"
	metricHyperVVIDRemotePhysicalPages       = "windows_hyperv_vid_remote_physical_pages"

	metricHypervVMDeviceBytesRead         = "windows_hyperv_vm_device_bytes_read"
	metricHypervVMDeviceBytesWritten      = "windows_hyperv_vm_device_bytes_written"
	metricHypervVMDeviceOperationsRead    = "windows_hyperv_vm_device_operations_read"
	metricHypervVMDeviceOperationsWritten = "windows_hyperv_vm_device_operations_written"
	metricHypervVMDeviceErrorCount        = "windows_hyperv_vm_device_error_count"

	metricHypervVMInterfaceBytesReceived          = "windows_hyperv_vm_interface_bytes_received"
	metricHypervVMInterfaceBytesSent              = "windows_hyperv_vm_interface_bytes_sent"
	metricHypervVMInterfacePacketsIncomingDropped = "windows_hyperv_vm_interface_packets_incoming_dropped"
	metricHypervVMInterfacePacketsOutgoingDropped = "windows_hyperv_vm_interface_packets_outgoing_dropped"
	metricHypervVMInterfacePacketsReceived        = "windows_hyperv_vm_interface_packets_received"
	metricHypervVMInterfacePacketsSent            = "windows_hyperv_vm_interface_packets_sent"

	metricHypervVSwitchBroadcastPacketsReceivedTotal         = "windows_hyperv_vswitch_broadcast_packets_received_total"
	metricHypervVSwitchBroadcastPacketsSentTotal             = "windows_hyperv_vswitch_broadcast_packets_sent_total"
	metricHypervVSwitchBytesReceivedTotal                    = "windows_hyperv_vswitch_bytes_received_total"
	metricHypervVSwitchBytesSentTotal                        = "windows_hyperv_vswitch_bytes_sent_total"
	metricHypervVSwitchPacketsReceivedTotal                  = "windows_hyperv_vswitch_packets_received_total"
	metricHypervVSwitchPacketsSentTotal                      = "windows_hyperv_vswitch_packets_sent_total"
	metricHypervVSwitchDirectedPacketsReceivedTotal          = "windows_hyperv_vswitch_directed_packets_received_total"
	metricHypervVSwitchDirectedPacketsSendTotal              = "windows_hyperv_vswitch_directed_packets_send_total"
	metricHypervVSwitchDroppedPacketsIncomingTotal           = "windows_hyperv_vswitch_dropped_packets_incoming_total"
	metricHypervVSwitchDroppedPacketsOutcomingTotal          = "windows_hyperv_vswitch_dropped_packets_outcoming_total"
	metricHypervVSwitchExtensionDroppedAttacksIncomingTotal  = "windows_hyperv_vswitch_extensions_dropped_packets_incoming_total"
	metricHypervVSwitchExtensionDroppedPacketsOutcomingTotal = "windows_hyperv_vswitch_extensions_dropped_packets_outcoming_total"
	metricHypervVSwitchLearnedMACAddressTotal                = "windows_hyperv_vswitch_learned_mac_addresses_total"
	metricHypervVSwitchMulticastPacketsReceivedTotal         = "windows_hyperv_vswitch_multicast_packets_received_total"
	metricHypervVSwitchMulticastPacketsSentTotal             = "windows_hyperv_vswitch_multicast_packets_sent_total"
	metricHypervVSwitchNumberOfSendChannelMovesTotal         = "windows_hyperv_vswitch_number_of_send_channel_moves_total"
	metricHypervVSwitchNumberOfVMQMovesTotal                 = "windows_hyperv_vswitch_number_of_vmq_moves_total"
	metricHypervVSwitchPacketsFloodedTotal                   = "windows_hyperv_vswitch_packets_flooded_total"
	metricHypervVSwitchPurgedMACAddresses                    = "windows_hyperv_vswitch_purged_mac_addresses_total"
)

func (w *Windows) collectHyperv(mx map[string]int64, pms prometheus.Series) {
	if !w.cache.collection[collectorHyperv] {
		w.cache.collection[collectorHyperv] = true
		w.addHypervCharts()
	}

	for _, v := range []string{
		metricHypervHealthOK,
		metricHypervHealthCritical,
		metricHypervRootPartition4KGPAPages,
		metricHypervRootPartition2MGPAPages,
		metricHypervRootPartition1GGPAPages,
		metricHypervRootPartition4KDevicePages,
		metricHypervRootPartition2MDevicePages,
		metricHypervRootPartition1GDevicePages,
		metricHypervRootPartitionGPASpaceModifications,
		metricHypervRootPartitionAddressSpace,
		metricHypervRootPartitionAttachedDevices,
		metricHypervRootPartitionDepositedPages,
		metricHypervRootPartitionPhysicalPagesAllocated,
		metricHypervRootPartitionDeviceDMAErrors,
		metricHypervRootPartitionDeviceInterruptErrors,
		metricHypervRootPartitionDeviceInterruptThrottleEvents,
		metricHypervRootPartitionIOTLBFlush,
		metricHypervRootPartitionVirtualTLBPages,
		metricHypervRootPartitionVirtualTLBFlushEntries,
	} {
		for _, pm := range pms.FindByName(v) {
			name := strings.TrimPrefix(pm.Name(), "windows_")
			mx[name] = int64(pm.Value)
		}
	}

	w.collectHypervVM(mx, pms)
	w.collectHypervVMDevices(mx, pms)
	w.collectHypervVMInterface(mx, pms)
	w.collectHypervVSwitch(mx, pms)
}

func (w *Windows) collectHypervVM(mx map[string]int64, pms prometheus.Series) {
	seen := make(map[string]bool)
	px := "hyperv_vm_"

	for _, v := range []string{
		metricHypervVMMemoryPhysical,
		metricHypervVMMemoryPhysicalGuestVisible,
		metricHypervVMMemoryPressureCurrent,
		metricsHypervVMCPUGuestRunTime,
		metricsHypervVMCPUHypervisorRunTime,
		metricsHypervVMCPURemoteRunTime,
	} {
		for _, pm := range pms.FindByName(v) {
			if vm := pm.Labels.Get("vm"); vm != "" {
				name := strings.TrimPrefix(pm.Name(), "windows_hyperv_vm")
				seen[vm] = true
				mx[px+hypervCleanName(vm)+name] += int64(pm.Value)
			}
		}
	}

	px = "hyperv_vid_"
	for _, v := range []string{
		metricHyperVVIDPhysicalPagesAllocated,
		metricHyperVVIDRemotePhysicalPages,
	} {
		for _, pm := range pms.FindByName(v) {
			if vm := pm.Labels.Get("vm"); vm != "" {
				name := strings.TrimPrefix(pm.Name(), "windows_hyperv_vid")
				seen[vm] = true
				mx[px+hypervCleanName(vm)+name] = int64(pm.Value)
			}
		}
	}

	for v := range seen {
		if !w.cache.hypervVMMem[v] {
			w.cache.hypervVMMem[v] = true
			w.addHypervVMCharts(v)
		}
	}
	for v := range w.cache.hypervVMMem {
		if !seen[v] {
			delete(w.cache.hypervVMMem, v)
			w.removeHypervVMCharts(v)
		}
	}
}

func (w *Windows) collectHypervVMDevices(mx map[string]int64, pms prometheus.Series) {
	seen := make(map[string]bool)
	px := "hyperv_vm_device_"

	for _, v := range []string{
		metricHypervVMDeviceBytesRead,
		metricHypervVMDeviceBytesWritten,
		metricHypervVMDeviceOperationsRead,
		metricHypervVMDeviceOperationsWritten,
		metricHypervVMDeviceErrorCount,
	} {
		for _, pm := range pms.FindByName(v) {
			if device := pm.Labels.Get("vm_device"); device != "" {
				name := strings.TrimPrefix(pm.Name(), "windows_hyperv_vm_device")
				seen[device] = true
				mx[px+hypervCleanName(device)+name] = int64(pm.Value)
			}
		}
	}

	for v := range seen {
		if !w.cache.hypervVMDevices[v] {
			w.cache.hypervVMDevices[v] = true
			w.addHypervVMDeviceCharts(v)
		}
	}
	for v := range w.cache.hypervVMDevices {
		if !seen[v] {
			delete(w.cache.hypervVMDevices, v)
			w.removeHypervVMDeviceCharts(v)
		}
	}
}

func (w *Windows) collectHypervVMInterface(mx map[string]int64, pms prometheus.Series) {
	seen := make(map[string]bool)
	px := "hyperv_vm_interface_"

	for _, v := range []string{
		metricHypervVMInterfaceBytesReceived,
		metricHypervVMInterfaceBytesSent,
		metricHypervVMInterfacePacketsIncomingDropped,
		metricHypervVMInterfacePacketsOutgoingDropped,
		metricHypervVMInterfacePacketsReceived,
		metricHypervVMInterfacePacketsSent,
	} {
		for _, pm := range pms.FindByName(v) {
			if iface := pm.Labels.Get("vm_interface"); iface != "" {
				name := strings.TrimPrefix(pm.Name(), "windows_hyperv_vm_interface")
				seen[iface] = true
				mx[px+hypervCleanName(iface)+name] = int64(pm.Value)
			}
		}
	}

	for v := range seen {
		if !w.cache.hypervVMInterfaces[v] {
			w.cache.hypervVMInterfaces[v] = true
			w.addHypervVMInterfaceCharts(v)
		}
	}
	for v := range w.cache.hypervVMInterfaces {
		if !seen[v] {
			delete(w.cache.hypervVMInterfaces, v)
			w.removeHypervVMInterfaceCharts(v)
		}
	}
}

func (w *Windows) collectHypervVSwitch(mx map[string]int64, pms prometheus.Series) {
	seen := make(map[string]bool)
	px := "hyperv_vswitch_"

	for _, v := range []string{
		metricHypervVSwitchBytesReceivedTotal,
		metricHypervVSwitchBytesSentTotal,
		metricHypervVSwitchPacketsReceivedTotal,
		metricHypervVSwitchPacketsSentTotal,
		metricHypervVSwitchDirectedPacketsReceivedTotal,
		metricHypervVSwitchDirectedPacketsSendTotal,
		metricHypervVSwitchBroadcastPacketsReceivedTotal,
		metricHypervVSwitchBroadcastPacketsSentTotal,
		metricHypervVSwitchMulticastPacketsReceivedTotal,
		metricHypervVSwitchMulticastPacketsSentTotal,
		metricHypervVSwitchDroppedPacketsIncomingTotal,
		metricHypervVSwitchDroppedPacketsOutcomingTotal,
		metricHypervVSwitchExtensionDroppedAttacksIncomingTotal,
		metricHypervVSwitchExtensionDroppedPacketsOutcomingTotal,
		metricHypervVSwitchPacketsFloodedTotal,
		metricHypervVSwitchLearnedMACAddressTotal,
		metricHypervVSwitchPurgedMACAddresses,
		metricHypervVSwitchNumberOfSendChannelMovesTotal,
		metricHypervVSwitchNumberOfVMQMovesTotal,
	} {
		for _, pm := range pms.FindByName(v) {
			if vswitch := pm.Labels.Get("vswitch"); vswitch != "" {
				name := strings.TrimPrefix(pm.Name(), "windows_hyperv_vswitch")
				seen[vswitch] = true
				mx[px+hypervCleanName(vswitch)+name] = int64(pm.Value)
			}
		}
	}

	for v := range seen {
		if !w.cache.hypervVswitch[v] {
			w.cache.hypervVswitch[v] = true
			w.addHypervVSwitchCharts(v)
		}
	}
	for v := range w.cache.hypervVswitch {
		if !seen[v] {
			delete(w.cache.hypervVswitch, v)
			w.removeHypervVSwitchCharts(v)
		}
	}
}

var hypervNameReplacer = strings.NewReplacer(" ", "_", "?", "_", ":", "_", ".", "_")

func hypervCleanName(name string) string {
	name = hypervNameReplacer.Replace(name)
	return strings.ToLower(name)
}
