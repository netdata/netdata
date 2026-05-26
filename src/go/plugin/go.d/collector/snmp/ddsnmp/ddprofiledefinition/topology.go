// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).

package ddprofiledefinition

type TopologyKind string

const (
	KindLldpLocPort          TopologyKind = "lldp_loc_port"
	KindLldpLocManAddr       TopologyKind = "lldp_loc_man_addr"
	KindLldpRem              TopologyKind = "lldp_rem"
	KindLldpRemManAddr       TopologyKind = "lldp_rem_man_addr"
	KindLldpRemManAddrCompat TopologyKind = "lldp_rem_man_addr_compat"
	KindCdpCache             TopologyKind = "cdp_cache"
	KindIfName               TopologyKind = "if_name"
	KindIfStatus             TopologyKind = "if_status"
	KindIfDuplex             TopologyKind = "if_duplex"
	KindIpIfIndex            TopologyKind = "ip_if_index"
	KindBridgePortIfIndex    TopologyKind = "bridge_port_if_index"
	KindFdbEntry             TopologyKind = "fdb_entry"
	KindQbridgeFdbEntry      TopologyKind = "qbridge_fdb_entry"
	KindQbridgeVlanEntry     TopologyKind = "qbridge_vlan_entry"
	KindStpPort              TopologyKind = "stp_port"
	KindVtpVlan              TopologyKind = "vtp_vlan"
	KindArpEntry             TopologyKind = "arp_entry"
	KindArpLegacyEntry       TopologyKind = "arp_legacy_entry"
)

var validTopologyKinds = map[TopologyKind]struct{}{
	KindLldpLocPort:          {},
	KindLldpLocManAddr:       {},
	KindLldpRem:              {},
	KindLldpRemManAddr:       {},
	KindLldpRemManAddrCompat: {},
	KindCdpCache:             {},
	KindIfName:               {},
	KindIfStatus:             {},
	KindIfDuplex:             {},
	KindIpIfIndex:            {},
	KindBridgePortIfIndex:    {},
	KindFdbEntry:             {},
	KindQbridgeFdbEntry:      {},
	KindQbridgeVlanEntry:     {},
	KindStpPort:              {},
	KindVtpVlan:              {},
	KindArpEntry:             {},
	KindArpLegacyEntry:       {},
}

func IsValidTopologyKind(kind TopologyKind) bool {
	_, ok := validTopologyKinds[kind]
	return ok
}

type TopologyConfig struct {
	Kind          TopologyKind `yaml:"kind" json:"kind"`
	MetricsConfig `yaml:",inline" json:",inline"`
}

func (c TopologyConfig) Clone() TopologyConfig {
	return TopologyConfig{
		Kind:          c.Kind,
		MetricsConfig: c.MetricsConfig.Clone(),
	}
}
