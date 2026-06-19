// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"fmt"
	"strings"

	catosdk "github.com/catonetworks/cato-go-sdk"
)

func mergeMetrics(metrics *catosdk.AccountMetrics, sites map[string]*siteState) []string {
	var issues []string
	for _, rawSite := range metrics.GetAccountMetrics().GetSites() {
		siteID := derefZero(rawSite.GetID())
		if siteID == "" {
			continue
		}

		site := sites[siteID]
		if site == nil {
			site = &siteState{
				ID:         siteID,
				Name:       siteDisplayName(siteID, nil, "", derefZero(rawSite.GetName())),
				Interfaces: make(map[string]*interfaceState),
			}
			sites[siteID] = site
		}
		if site.Name == "" {
			site.Name = siteDisplayName(siteID, nil, "", derefZero(rawSite.GetName()))
		}

		site.Metrics = mergeCatoTrafficMetrics(site.Metrics, rawSite.GetMetrics())
		for _, rawIface := range rawSite.GetInterfaces() {
			iface, ifaceIssues := normalizeMetricsInterface(rawIface)
			issues = append(issues, ifaceIssues...)
			if iface.Name == "" {
				iface.Name = "all"
			}
			if strings.EqualFold(iface.Name, "all") {
				site.Metrics, _ = mergeRawInterfaceTrafficMetrics(site.Metrics, rawIface)
			}
			key := interfaceKey(iface.ID, iface.Name)
			resolveMetricInterfaceDevice(site, &iface)
			if iface.ID == "" && iface.DeviceID != "" {
				key = snapshotInterfaceKey(iface.DeviceID, iface.ID, iface.Name)
			}
			existing := site.Interfaces[key]
			if existing == nil {
				site.Interfaces[key] = &iface
				continue
			}
			if iface.DeviceID == "" {
				iface.DeviceID = existing.DeviceID
			}
			if iface.DeviceName == "" {
				iface.DeviceName = existing.DeviceName
			}
			existing.Metrics = iface.Metrics
			existing.DeviceID = iface.DeviceID
			existing.DeviceName = iface.DeviceName
			existing.DeviceSocketID = iface.DeviceSocketID
			existing.DeviceSocketSerial = iface.DeviceSocketSerial
			if existing.PopName == "" {
				existing.PopName = iface.PopName
			}
			if existing.Type == "" {
				existing.Type = iface.Type
			}
			if existing.TunnelRemoteIP == "" {
				existing.TunnelRemoteIP = iface.TunnelRemoteIP
			}
		}
	}
	return issues
}

func normalizeMetricsInterface(raw *catosdk.AccountMetrics_AccountMetrics_Sites_Interfaces) (interfaceState, []string) {
	iface := interfaceState{
		Name:           derefZero(raw.GetName()),
		TunnelRemoteIP: derefZero(raw.GetRemoteIP()),
	}
	var issues []string
	if info := raw.GetInterfaceInfo(); info != nil {
		iface.ID = info.GetID()
		if iface.Name == "" {
			iface.Name = derefZero(info.GetName())
		}
		iface.DestType = derefZero(info.GetDestType())
		iface.UpstreamBandwidth = derefZero(info.GetUpstreamBandwidth())
		iface.DownstreamBandwidth = derefZero(info.GetDownstreamBandwidth())
	}
	if socket := raw.GetSocketInfo(); socket != nil {
		iface.DeviceSocketID = derefZero(socket.GetID())
		iface.DeviceSocketSerial = derefZero(socket.GetSerial())
		if iface.Type == "" && socket.GetPlatform() != nil {
			iface.Type = fmt.Sprint(*socket.GetPlatform())
		}
	}
	if remote := raw.GetRemoteIPInfo(); remote != nil && iface.TunnelRemoteIP == "" {
		iface.TunnelRemoteIP = derefZero(remote.GetIP())
	}

	var metricIssues []string
	iface.Metrics, metricIssues = mergeRawInterfaceTrafficMetrics(iface.Metrics, raw)
	issues = append(issues, metricIssues...)

	return iface, issues
}

func mergeRawInterfaceTrafficMetrics(base trafficMetrics, raw *catosdk.AccountMetrics_AccountMetrics_Sites_Interfaces) (trafficMetrics, []string) {
	if raw == nil {
		return base, nil
	}
	var issues []string
	base = mergeCatoTrafficMetrics(base, raw.GetMetrics())
	for _, ts := range raw.GetTimeseries() {
		value, ok := latestTimeseriesValue(ts.GetData())
		if !applyTimeseriesMetric(&base, ts.GetLabel(), value, ok) {
			issues = append(issues, normalizationIssueUnknownTimeseriesLabel)
		}
	}
	return base, issues
}

type catoTrafficMetrics interface {
	GetBytesUpstream() *float64
	GetBytesDownstream() *float64
	GetLostUpstreamPcnt() *float64
	GetLostDownstreamPcnt() *float64
	GetJitterUpstream() *float64
	GetJitterDownstream() *float64
	GetPacketsDiscardedUpstream() *float64
	GetPacketsDiscardedDownstream() *float64
	GetRtt() *int64
}

func mergeCatoTrafficMetrics(base trafficMetrics, raw catoTrafficMetrics) trafficMetrics {
	if raw == nil {
		return base
	}
	if raw.GetBytesUpstream() != nil {
		base.present |= trafficMetricBytesUpstreamMax
		base.BytesUpstreamMax = *raw.GetBytesUpstream()
	}
	if raw.GetBytesDownstream() != nil {
		base.present |= trafficMetricBytesDownstreamMax
		base.BytesDownstreamMax = *raw.GetBytesDownstream()
	}
	if raw.GetLostUpstreamPcnt() != nil {
		base.present |= trafficMetricLostUpstreamPercent
		base.LostUpstreamPercent = *raw.GetLostUpstreamPcnt()
	}
	if raw.GetLostDownstreamPcnt() != nil {
		base.present |= trafficMetricLostDownstreamPercent
		base.LostDownstreamPercent = *raw.GetLostDownstreamPcnt()
	}
	if raw.GetJitterUpstream() != nil {
		base.present |= trafficMetricJitterUpstreamMS
		base.JitterUpstreamMS = *raw.GetJitterUpstream()
	}
	if raw.GetJitterDownstream() != nil {
		base.present |= trafficMetricJitterDownstreamMS
		base.JitterDownstreamMS = *raw.GetJitterDownstream()
	}
	if raw.GetPacketsDiscardedUpstream() != nil {
		base.present |= trafficMetricPacketsDiscardedUpstream
		base.PacketsDiscardedUpstream = *raw.GetPacketsDiscardedUpstream()
	}
	if raw.GetPacketsDiscardedDownstream() != nil {
		base.present |= trafficMetricPacketsDiscardedDownstream
		base.PacketsDiscardedDownstream = *raw.GetPacketsDiscardedDownstream()
	}
	if raw.GetRtt() != nil {
		base.present |= trafficMetricRTTMS
		base.RTTMS = float64(*raw.GetRtt())
	}
	return base
}

func latestTimeseriesValue(data [][]float64) (float64, bool) {
	for i := len(data) - 1; i >= 0; i-- {
		if len(data[i]) >= 2 {
			return data[i][1], true
		}
	}
	return 0, false
}

func applyTimeseriesMetric(m *trafficMetrics, label string, value float64, ok bool) bool {
	if !ok {
		return true
	}
	switch strings.TrimSpace(label) {
	case "bytesUpstream", "bytesUpstreamMax":
		m.present |= trafficMetricBytesUpstreamMax
		m.BytesUpstreamMax = value
	case "bytesDownstream", "bytesDownstreamMax":
		m.present |= trafficMetricBytesDownstreamMax
		m.BytesDownstreamMax = value
	case "lostUpstreamPcnt":
		m.present |= trafficMetricLostUpstreamPercent
		m.LostUpstreamPercent = value
	case "lostDownstreamPcnt":
		m.present |= trafficMetricLostDownstreamPercent
		m.LostDownstreamPercent = value
	case "jitterUpstream":
		m.present |= trafficMetricJitterUpstreamMS
		m.JitterUpstreamMS = value
	case "jitterDownstream":
		m.present |= trafficMetricJitterDownstreamMS
		m.JitterDownstreamMS = value
	case "packetsDiscardedUpstream":
		m.present |= trafficMetricPacketsDiscardedUpstream
		m.PacketsDiscardedUpstream = value
	case "packetsDiscardedDownstream":
		m.present |= trafficMetricPacketsDiscardedDownstream
		m.PacketsDiscardedDownstream = value
	case "rtt":
		m.present |= trafficMetricRTTMS
		m.RTTMS = value
	case "lastMileLatency":
		m.present |= trafficMetricLastMileLatencyMS
		m.LastMileLatencyMS = value
	case "lastMilePacketLoss":
		m.present |= trafficMetricLastMilePacketLossPercent
		m.LastMilePacketLossPercent = value
	default:
		return false
	}
	return true
}
