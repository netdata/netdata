// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"fmt"
	"strconv"
	"strings"

	catosdk "github.com/catonetworks/cato-go-sdk"
	catomodels "github.com/catonetworks/cato-go-sdk/models"
	catoscalars "github.com/catonetworks/cato-go-sdk/scalars"
)

func ptrString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func ptrBool(v *bool) bool {
	return v != nil && *v
}

func ptrInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func normalizeName(v string) string {
	return strings.TrimSpace(v)
}

func normalizeStatus(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "unknown"
	}
	return strings.ToLower(strings.ReplaceAll(v, " ", "_"))
}

func connectivityStatusString(v *catomodels.ConnectivityStatus) string {
	if v == nil {
		return ""
	}
	return string(*v)
}

func operationalStatusString(v *catoscalars.OperationalStatus) string {
	if v == nil {
		return ""
	}
	return v.GetString()
}

func siteDisplayName(siteID string, siteNames map[string]string, infoName, fallbackName string) string {
	switch {
	case normalizeName(infoName) != "":
		return normalizeName(infoName)
	case normalizeName(fallbackName) != "":
		return normalizeName(fallbackName)
	case normalizeName(siteNames[siteID]) != "":
		return normalizeName(siteNames[siteID])
	default:
		return siteID
	}
}

func normalizeSnapshot(snapshot *catosdk.AccountSnapshot, siteNames map[string]string) (map[string]*siteState, []string) {
	out := make(map[string]*siteState)
	var order []string

	for _, raw := range snapshot.GetAccountSnapshot().GetSites() {
		siteID := ptrString(raw.GetID())
		if siteID == "" {
			continue
		}

		var (
			infoName    string
			description string
			countryCode string
			countryName string
			region      string
			siteType    string
			connType    string
		)
		if info := raw.GetInfoSiteSnapshot(); info != nil {
			infoName = ptrString(info.GetName())
			description = ptrString(info.GetDescription())
			countryCode = ptrString(info.GetCountryCode())
			countryName = ptrString(info.GetCountryName())
			region = ptrString(info.GetRegion())
			if info.GetType() != nil {
				siteType = fmt.Sprint(*info.GetType())
			}
			if info.GetConnType() != nil {
				connType = fmt.Sprint(*info.GetConnType())
			}
		}
		site := &siteState{
			ID:                 siteID,
			Name:               siteDisplayName(siteID, siteNames, infoName, ""),
			Description:        description,
			ConnectivityStatus: normalizeStatus(connectivityStatusString(raw.GetConnectivityStatusSiteSnapshot())),
			OperationalStatus:  normalizeStatus(operationalStatusString(raw.GetOperationalStatusSiteSnapshot())),
			PopName:            ptrString(raw.GetPopName()),
			CountryCode:        countryCode,
			CountryName:        countryName,
			Region:             region,
			SiteType:           siteType,
			ConnectionType:     connType,
			LastConnected:      ptrString(raw.GetLastConnected()),
			ConnectedSince:     ptrString(raw.GetConnectedSince()),
			HostCount:          ptrInt64(raw.GetHostCount()),
			Interfaces:         make(map[string]*interfaceState),
		}

		for _, dev := range raw.GetDevices() {
			device := deviceState{
				ID:             ptrString(dev.GetID()),
				Name:           ptrString(dev.GetName()),
				Type:           ptrString(dev.GetType()),
				Connected:      ptrBool(dev.GetConnected()),
				HaRole:         ptrString(dev.GetHaRole()),
				InternalIP:     ptrString(dev.GetInternalIP()),
				LastPopName:    ptrString(dev.GetLastPopName()),
				ConnectedSince: ptrString(dev.GetConnectedSince()),
			}
			if socket := dev.GetSocketInfo(); socket != nil {
				device.SocketID = ptrString(socket.GetID())
				device.SocketSerial = ptrString(socket.GetSerial())
				device.SocketVersion = ptrString(socket.GetVersion())
			}
			site.Devices = append(site.Devices, device)

			linkStateByID := make(map[string]*catosdk.AccountSnapshot_AccountSnapshot_Sites_Devices_InterfacesLinkState)
			for _, linkState := range dev.GetInterfacesLinkState() {
				if id := ptrString(linkState.GetID()); id != "" {
					linkStateByID[id] = linkState
				}
			}
			for _, rawIface := range dev.GetInterfaces() {
				iface := normalizeSnapshotInterface(rawIface)
				if iface.ID == "" && iface.Name == "" {
					continue
				}
				if linkState := linkStateByID[iface.ID]; linkState != nil {
					iface.LinkUp = ptrBool(linkState.GetUp())
				}
				key := interfaceKey(iface.ID, iface.Name)
				site.Interfaces[key] = &iface
			}
		}

		out[siteID] = site
		order = append(order, siteID)
	}

	return out, order
}

func normalizeSnapshotInterface(raw *catosdk.AccountSnapshot_AccountSnapshot_Sites_Devices_Interfaces) interfaceState {
	iface := interfaceState{
		ID:             ptrString(raw.GetID()),
		Name:           ptrString(raw.GetName()),
		Type:           ptrString(raw.GetType()),
		Connected:      ptrBool(raw.GetConnected()),
		PopName:        ptrString(raw.GetPopName()),
		TunnelRemoteIP: ptrString(raw.GetTunnelRemoteIP()),
		TunnelUptime:   ptrInt64(raw.GetTunnelUptime()),
		PhysicalPort:   ptrInt64(raw.GetPhysicalPort()),
	}
	if info := raw.GetInfoInterfaceSnapshot(); info != nil {
		if iface.ID == "" {
			iface.ID = info.GetID()
		}
		if iface.Name == "" {
			iface.Name = ptrString(info.GetName())
		}
		iface.DestType = ptrString(info.GetDestType())
		iface.UpstreamBandwidth = ptrInt64(info.GetUpstreamBandwidth())
		iface.DownstreamBandwidth = ptrInt64(info.GetDownstreamBandwidth())
	}
	return iface
}

func mergeMetrics(metrics *catosdk.AccountMetrics, sites map[string]*siteState) []string {
	var issues []string
	for _, rawSite := range metrics.GetAccountMetrics().GetSites() {
		siteID := ptrString(rawSite.GetID())
		if siteID == "" {
			continue
		}

		site := sites[siteID]
		if site == nil {
			site = &siteState{
				ID:         siteID,
				Name:       siteDisplayName(siteID, nil, "", ptrString(rawSite.GetName())),
				Interfaces: make(map[string]*interfaceState),
			}
			sites[siteID] = site
		}
		if site.Name == "" {
			site.Name = siteDisplayName(siteID, nil, "", ptrString(rawSite.GetName()))
		}

		site.Metrics = mergeSiteMetrics(site.Metrics, rawSite.GetMetrics())
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
			existing := site.Interfaces[key]
			if existing == nil {
				site.Interfaces[key] = &iface
				continue
			}
			existing.Metrics = iface.Metrics
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
		Name:           ptrString(raw.GetName()),
		TunnelRemoteIP: ptrString(raw.GetRemoteIP()),
	}
	var issues []string
	if info := raw.GetInterfaceInfo(); info != nil {
		iface.ID = info.GetID()
		if iface.Name == "" {
			iface.Name = ptrString(info.GetName())
		}
		iface.DestType = ptrString(info.GetDestType())
		iface.UpstreamBandwidth = ptrInt64(info.GetUpstreamBandwidth())
		iface.DownstreamBandwidth = ptrInt64(info.GetDownstreamBandwidth())
	}
	if socket := raw.GetSocketInfo(); socket != nil && iface.Type == "" && socket.GetPlatform() != nil {
		iface.Type = fmt.Sprint(*socket.GetPlatform())
	}
	if remote := raw.GetRemoteIPInfo(); remote != nil && iface.TunnelRemoteIP == "" {
		iface.TunnelRemoteIP = ptrString(remote.GetIP())
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
	base = mergeInterfaceMetrics(base, raw.GetMetrics())
	for _, ts := range raw.GetTimeseries() {
		value, ok := latestTimeseriesValue(ts.GetData())
		if !applyTimeseriesMetric(&base, ts.GetLabel(), value, ok) {
			issues = append(issues, normalizationIssueUnknownTimeseriesLabel)
		}
	}
	return base, issues
}

func mergeSiteMetrics(base trafficMetrics, raw *catosdk.AccountMetrics_AccountMetrics_Sites_Metrics) trafficMetrics {
	return mergeCatoTrafficMetrics(base, raw)
}

func mergeInterfaceMetrics(base trafficMetrics, raw *catosdk.AccountMetrics_AccountMetrics_Sites_Interfaces_Metrics) trafficMetrics {
	return mergeCatoTrafficMetrics(base, raw)
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

func normalizeBGP(raw []*catosdk.SiteBgpStatusResult) ([]bgpPeerState, []string) {
	peers := make([]bgpPeerState, 0, len(raw))
	peerIndexes := make(map[string]int)
	var issues []string
	for _, v := range raw {
		if v == nil {
			issues = append(issues, normalizationIssueEmptyPeer)
			continue
		}
		if isEmptyBGPPeerResult(v) {
			issues = append(issues, normalizationIssueEmptyPeer)
			continue
		}
		if !hasBGPPeerRemoteIdentity(v) {
			issues = append(issues, normalizationIssueEmptyPeer)
			continue
		}
		routesCount, ok := parseInt64(v.RoutesCount)
		if !ok {
			issues = append(issues, normalizationIssueParseInt)
		}
		routesCountLimit, ok := parseInt64(v.RoutesCountLimit)
		if !ok {
			issues = append(issues, normalizationIssueParseInt)
		}
		peer := bgpPeerState{
			RemoteIP:                 v.RemoteIP,
			RemoteASN:                v.RemoteASN,
			LocalIP:                  v.LocalIP,
			LocalASN:                 v.LocalASN,
			BGPSession:               normalizeStatus(v.BGPSession),
			IncomingState:            normalizeStatus(v.IncomingConnection.State),
			OutgoingState:            normalizeStatus(v.OutgoingConnection.State),
			RoutesCount:              routesCount,
			RoutesCountLimit:         routesCountLimit,
			RoutesCountLimitExceeded: v.RoutesCountLimitExceeded,
			RIBOutRoutes:             int64(len(v.RIBOut)),
		}
		key := bgpPeerKey(peer.RemoteIP, peer.RemoteASN)
		if idx, ok := peerIndexes[key]; ok {
			peers[idx] = peer
			continue
		}
		peerIndexes[key] = len(peers)
		peers = append(peers, peer)
	}
	return peers, issues
}

func bgpPeerKey(remoteIP, remoteASN string) string {
	return strings.TrimSpace(remoteIP) + "\x00" + strings.TrimSpace(remoteASN)
}

func hasBGPPeerRemoteIdentity(v *catosdk.SiteBgpStatusResult) bool {
	return strings.TrimSpace(v.RemoteIP) != "" || strings.TrimSpace(v.RemoteASN) != ""
}

func isEmptyBGPPeerResult(v *catosdk.SiteBgpStatusResult) bool {
	return strings.TrimSpace(v.RemoteIP) == "" &&
		strings.TrimSpace(v.RemoteASN) == "" &&
		strings.TrimSpace(v.LocalIP) == "" &&
		strings.TrimSpace(v.LocalASN) == "" &&
		strings.TrimSpace(v.BGPSession) == "" &&
		strings.TrimSpace(v.IncomingConnection.State) == "" &&
		strings.TrimSpace(v.OutgoingConnection.State) == "" &&
		strings.TrimSpace(v.RoutesCount) == "" &&
		strings.TrimSpace(v.RoutesCountLimit) == "" &&
		len(v.RIBOut) == 0
}

func parseInt64(v string) (int64, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, true
	}
	n, err := strconv.ParseInt(v, 10, 64)
	return n, err == nil
}

func interfaceKey(id, name string) string {
	if strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id)
	}
	if strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return "all"
}
