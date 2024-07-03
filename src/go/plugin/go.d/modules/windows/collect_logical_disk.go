// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
)

const (
	metricLDReadBytesTotal    = "windows_logical_disk_read_bytes_total"
	metricLDWriteBytesTotal   = "windows_logical_disk_write_bytes_total"
	metricLDReadsTotal        = "windows_logical_disk_reads_total"
	metricLDWritesTotal       = "windows_logical_disk_writes_total"
	metricLDSizeBytes         = "windows_logical_disk_size_bytes"
	metricLDFreeBytes         = "windows_logical_disk_free_bytes"
	metricLDReadLatencyTotal  = "windows_logical_disk_read_latency_seconds_total"
	metricLDWriteLatencyTotal = "windows_logical_disk_write_latency_seconds_total"
)

func (w *Windows) collectLogicalDisk(mx map[string]int64, pms prometheus.Series) {
	seen := make(map[string]bool)
	px := "logical_disk_"
	for _, pm := range pms.FindByName(metricLDReadBytesTotal) {
		vol := pm.Labels.Get("volume")
		if vol != "" && !strings.HasPrefix(vol, "HarddiskVolume") {
			seen[vol] = true
			mx[px+vol+"_read_bytes_total"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricLDWriteBytesTotal) {
		vol := pm.Labels.Get("volume")
		if vol != "" && !strings.HasPrefix(vol, "HarddiskVolume") {
			seen[vol] = true
			mx[px+vol+"_write_bytes_total"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricLDReadsTotal) {
		vol := pm.Labels.Get("volume")
		if vol != "" && !strings.HasPrefix(vol, "HarddiskVolume") {
			seen[vol] = true
			mx[px+vol+"_reads_total"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricLDWritesTotal) {
		vol := pm.Labels.Get("volume")
		if vol != "" && !strings.HasPrefix(vol, "HarddiskVolume") {
			seen[vol] = true
			mx[px+vol+"_writes_total"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricLDSizeBytes) {
		vol := pm.Labels.Get("volume")
		if vol != "" && !strings.HasPrefix(vol, "HarddiskVolume") {
			seen[vol] = true
			mx[px+vol+"_total_space"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricLDFreeBytes) {
		vol := pm.Labels.Get("volume")
		if vol != "" && !strings.HasPrefix(vol, "HarddiskVolume") {
			seen[vol] = true
			mx[px+vol+"_free_space"] = int64(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metricLDReadLatencyTotal) {
		vol := pm.Labels.Get("volume")
		if vol != "" && !strings.HasPrefix(vol, "HarddiskVolume") {
			seen[vol] = true
			mx[px+vol+"_read_latency"] = int64(pm.Value * precision)
		}
	}
	for _, pm := range pms.FindByName(metricLDWriteLatencyTotal) {
		vol := pm.Labels.Get("volume")
		if vol != "" && !strings.HasPrefix(vol, "HarddiskVolume") {
			seen[vol] = true
			mx[px+vol+"_write_latency"] = int64(pm.Value * precision)
		}
	}

	for disk := range seen {
		if !w.cache.volumes[disk] {
			w.cache.volumes[disk] = true
			w.addDiskCharts(disk)
		}
		mx[px+disk+"_used_space"] = mx[px+disk+"_total_space"] - mx[px+disk+"_free_space"]
	}
	for disk := range w.cache.volumes {
		if !seen[disk] {
			delete(w.cache.volumes, disk)
			w.removeDiskCharts(disk)
		}
	}
}
