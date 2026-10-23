// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"sort"
	"strconv"
	"strings"

	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
)

func buildCatoTopologyLabels(dict *topologyv1.StringDictionary, accountID string, sites map[string]*siteState, order []string, indexes map[string]int) (topologyv1.Table, error) {
	table := topologyv1.NewTableBuilder(catoTopologyLabelColumns()...)
	add := func(actor int, key, value, kind string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		table.Add(actor, dict.Ref(key), dict.Ref(value), dict.Ref(topologySource), dict.Ref(kind))
	}
	siteContext := func(actor int, site *siteState) {
		add(actor, "account_id", accountID, "identity")
		add(actor, "site_id", site.ID, "identity")
		add(actor, "site_name", site.Name, "hierarchy")
		add(actor, "country_code", site.CountryCode, "location")
		add(actor, "country_name", site.CountryName, "location")
		add(actor, "region", site.Region, "location")
	}
	for _, id := range order {
		site := sites[id]
		if site == nil {
			continue
		}
		actor := indexes[catoSiteActorID(site.ID)]
		siteContext(actor, site)
		add(actor, "name", site.Name, "identity")
		add(actor, "description", site.Description, "metadata")
		add(actor, "pop_name", site.PopName, "metadata")
		add(actor, "site_type", site.SiteType, "metadata")
		add(actor, "connection_type", site.ConnectionType, "metadata")
		add(actor, "connectivity_status", site.ConnectivityStatus, "status")
		add(actor, "operational_status", site.OperationalStatus, "status")
		add(actor, "snapshot_collection", collectionStatusLabel(site.SnapshotCollection), "status")
		add(actor, "metrics_collection", collectionStatusLabel(site.MetricsCollection), "status")
		add(actor, "bgp_collection", collectionStatusLabel(site.BGPCollection), "status")
		if site.HostCount != nil {
			add(actor, "host_count", strconv.FormatInt(*site.HostCount, 10), "metadata")
		}
		for _, dev := range topologyDevices(site.Devices) {
			actor, ok := indexes[catoDeviceActorID(site.ID, stableDeviceID(dev))]
			if !ok {
				continue
			}
			siteContext(actor, site)
			add(actor, "name", topologyDeviceName(site, dev), "identity")
			add(actor, "source_name", dev.Name, "metadata")
			add(actor, "device_id", stableDeviceID(dev), "identity")
			add(actor, "identifier", dev.Identifier, "identity")
			add(actor, "socket_id", dev.SocketID, "identity")
			add(actor, "socket_serial", dev.SocketSerial, "identity")
			add(actor, "socket_version", dev.SocketVersion, "metadata")
			add(actor, "internal_ip", dev.InternalIP, "metadata")
			add(actor, "ha_role", dev.HaRole, "metadata")
			add(actor, "last_pop_name", dev.LastPopName, "metadata")
			if dev.Connected != nil {
				add(actor, "connected", strconv.FormatBool(*dev.Connected), "status")
			}
		}
		seen := make(map[int]bool)
		for _, peer := range site.BGPPeers {
			actor, ok := indexes[catoBGPPeerActorID(site.ID, peer.RemoteIP, peer.RemoteASN)]
			if !ok || seen[actor] {
				continue
			}
			seen[actor] = true
			siteContext(actor, site)
			add(actor, "remote_ip", peer.RemoteIP, "identity")
			add(actor, "remote_asn", peer.RemoteASN, "identity")
			add(actor, "bgp_session", peer.BGPSession, "status")
		}
	}
	// PoPs are shared by sites; labels belong to the account, not an arbitrary site.
	ids := make([]string, 0)
	for id := range indexes {
		if strings.HasPrefix(id, "cato:pop:") {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		actor := indexes[id]
		add(actor, "account_id", accountID, "identity")
		add(actor, "pop_name", strings.TrimPrefix(id, "cato:pop:"), "identity")
	}
	return table.Table()
}

func catoTopologyLabelColumns() []topologyv1.Column {
	return []topologyv1.Column{
		topologyv1.NewColumn("actor", "actor_ref", topologyv1.WithRole("reference")),
		topologyv1.NewColumn("key", "string_ref", topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("value", "string_ref", topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("source", "string_ref", topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("kind", "string_ref", topologyv1.WithDictionary("strings")),
	}
}

func collectionStatusLabel(status string) string {
	switch status {
	case "observed":
		return "Observed"
	case "empty":
		return "No peers reported"
	case "empty_cached":
		return "No peers reported (cached result)"
	case "partial":
		return "Partial observations"
	case "unavailable":
		return "Unavailable"
	default:
		return "Unknown"
	}
}
