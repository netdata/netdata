// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"

type TopologyKind = ddprofiledefinition.TopologyKind

const (
	KindLldpLocPort          = ddprofiledefinition.KindLldpLocPort
	KindLldpLocManAddr       = ddprofiledefinition.KindLldpLocManAddr
	KindLldpRem              = ddprofiledefinition.KindLldpRem
	KindLldpRemManAddr       = ddprofiledefinition.KindLldpRemManAddr
	KindLldpRemManAddrCompat = ddprofiledefinition.KindLldpRemManAddrCompat
	KindCdpCache             = ddprofiledefinition.KindCdpCache
	KindIfName               = ddprofiledefinition.KindIfName
	KindIfStatus             = ddprofiledefinition.KindIfStatus
	KindIfDuplex             = ddprofiledefinition.KindIfDuplex
	KindIpIfIndex            = ddprofiledefinition.KindIpIfIndex
	KindBridgePortIfIndex    = ddprofiledefinition.KindBridgePortIfIndex
	KindFdbEntry             = ddprofiledefinition.KindFdbEntry
	KindQbridgeFdbEntry      = ddprofiledefinition.KindQbridgeFdbEntry
	KindQbridgeVlanEntry     = ddprofiledefinition.KindQbridgeVlanEntry
	KindStpPort              = ddprofiledefinition.KindStpPort
	KindVtpVlan              = ddprofiledefinition.KindVtpVlan
	KindArpEntry             = ddprofiledefinition.KindArpEntry
	KindArpLegacyEntry       = ddprofiledefinition.KindArpLegacyEntry
)
