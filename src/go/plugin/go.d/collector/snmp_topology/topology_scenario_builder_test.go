// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"hash/fnv"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	topologyapi "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1test"
	"github.com/stretchr/testify/require"
)

const (
	topologyScenarioProducerScopeID = "scenario-producer"
	topologyScenarioSysDescr        = "Synthetic SNMP topology scenario device"
)

var topologyScenarioCollectedAt = time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)

type topologyScenario struct {
	name   string
	opts   topologyoptions.QueryOptions
	devs   []*topologyScenarioDevice
	lldp   []topologyScenarioPortPair
	cdp    []topologyScenarioPortPair
	stp    []topologyScenarioPortPair
	ospf   []topologyScenarioPortPair
	bgp    []topologyScenarioBGPAdjacency
	fdbARP []topologyScenarioFDBARP
}

type topologyScenarioDevice struct {
	scenario     *topologyScenario
	name         string
	actorType    string
	mgmtIP       string
	chassisMAC   string
	sysObjectID  string
	routerID     string
	localAS      string
	capabilities string
	ports        []*topologyScenarioPort
}

type topologyScenarioPort struct {
	device     *topologyScenarioDevice
	name       string
	ifIndex    int
	bridgePort string
	mac        string
	ip         string
	netmask    string
}

type topologyScenarioPortPair struct {
	a *topologyScenarioPort
	b *topologyScenarioPort
}

type topologyScenarioBGPAdjacency struct {
	a               *topologyScenarioDevice
	b               *topologyScenarioDevice
	routingInstance string
}

type topologyScenarioFDBARP struct {
	port *topologyScenarioPort
	mac  string
	ip   string
}

func newTopologyScenario(name string) *topologyScenario {
	return &topologyScenario{
		name: name,
		opts: defaultTopologyQueryOptionsForTest(),
	}
}

func (s *topologyScenario) WithOptions(fn func(*topologyoptions.QueryOptions)) *topologyScenario {
	if fn != nil {
		fn(&s.opts)
	}
	return s
}

func (s *topologyScenario) Router(name, mgmtIP, chassisMAC, routerID, localAS string) *topologyScenarioDevice {
	return s.device("router", name, mgmtIP, chassisMAC, routerID, localAS)
}

func (s *topologyScenario) Switch(name, mgmtIP, chassisMAC string) *topologyScenarioDevice {
	return s.device("switch", name, mgmtIP, chassisMAC, "", "")
}

func (s *topologyScenario) Device(name, mgmtIP, chassisMAC string) *topologyScenarioDevice {
	return s.device("device", name, mgmtIP, chassisMAC, "", "")
}

func (s *topologyScenario) device(actorType, name, mgmtIP, chassisMAC, routerID, localAS string) *topologyScenarioDevice {
	dev := &topologyScenarioDevice{
		scenario:     s,
		name:         name,
		actorType:    actorType,
		mgmtIP:       mgmtIP,
		chassisMAC:   topologyutil.NormalizeMAC(chassisMAC),
		sysObjectID:  fmt.Sprintf("1.3.6.1.4.1.8072.3.2.%d", len(s.devs)+1),
		routerID:     routerID,
		localAS:      localAS,
		capabilities: topologyScenarioCapabilities(actorType),
	}
	s.devs = append(s.devs, dev)
	return dev
}

func (d *topologyScenarioDevice) Port(name string, ifIndex int) *topologyScenarioPort {
	port := &topologyScenarioPort{
		device:     d,
		name:       name,
		ifIndex:    ifIndex,
		bridgePort: strconv.Itoa(ifIndex),
		mac:        topologyScenarioPortMAC(d.chassisMAC, ifIndex),
	}
	d.ports = append(d.ports, port)
	return port
}

func (p *topologyScenarioPort) IPv4(cidr string) *topologyScenarioPort {
	ip, netmask := topologyScenarioIPv4CIDR(cidr)
	p.ip = ip
	p.netmask = netmask
	return p
}

func (p *topologyScenarioPort) BridgePort(value int) *topologyScenarioPort {
	p.bridgePort = strconv.Itoa(value)
	return p
}

func (p *topologyScenarioPort) MAC(value string) *topologyScenarioPort {
	if mac := topologyutil.NormalizeMAC(value); mac != "" {
		p.mac = mac
	}
	return p
}

func (s *topologyScenario) LLDP(a, b *topologyScenarioPort) *topologyScenario {
	s.lldp = append(s.lldp, topologyScenarioPortPair{a: a, b: b}, topologyScenarioPortPair{a: b, b: a})
	return s
}

func (s *topologyScenario) CDP(a, b *topologyScenarioPort) *topologyScenario {
	s.cdp = append(s.cdp, topologyScenarioPortPair{a: a, b: b}, topologyScenarioPortPair{a: b, b: a})
	return s
}

func (s *topologyScenario) STP(a, b *topologyScenarioPort) *topologyScenario {
	s.stp = append(s.stp, topologyScenarioPortPair{a: a, b: b}, topologyScenarioPortPair{a: b, b: a})
	return s
}

func (s *topologyScenario) OSPF(a, b *topologyScenarioPort) *topologyScenario {
	s.ospf = append(s.ospf, topologyScenarioPortPair{a: a, b: b}, topologyScenarioPortPair{a: b, b: a})
	return s
}

func (s *topologyScenario) BGP(a, b *topologyScenarioDevice, routingInstance string) *topologyScenario {
	s.bgp = append(s.bgp, topologyScenarioBGPAdjacency{a: a, b: b, routingInstance: routingInstance})
	return s
}

func (s *topologyScenario) FDBARP(port *topologyScenarioPort, mac, ip string) *topologyScenario {
	s.fdbARP = append(s.fdbARP, topologyScenarioFDBARP{port: port, mac: mac, ip: ip})
	return s
}

func (s *topologyScenario) render(t testing.TB) topologyapi.Data {
	t.Helper()

	registry := newTopologyRegistry()
	registry.producerScopeID = topologyScenarioProducerScopeID
	for _, dev := range s.devs {
		registry.register(s.cacheForDevice(t, dev))
	}

	payload, ok, err := (funcDepsAdapter{registry: registry}).Snapshot(s.opts)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, topologyv1test.ValidateData(payload))
	return payload
}

func (s *topologyScenario) cacheForDevice(t testing.TB, dev *topologyScenarioDevice) *topologyCache {
	t.Helper()

	cache := newTestTopologyCache(ddsnmp.DeviceConnectionInfo{
		Hostname:    dev.mgmtIP,
		SysObjectID: dev.sysObjectID,
		SysName:     dev.name,
		SysDescr:    topologyScenarioSysDescr,
	})
	cache.updateTime = topologyScenarioCollectedAt
	cache.updateTopologySysUptime(3600)

	pm := &ddsnmp.ProfileMetrics{
		DeviceMetadata:  s.deviceMetadata(dev),
		TopologyMetrics: s.topologyMetricsForDevice(dev),
		BGPRows:         s.bgpRowsForDevice(dev),
	}
	cache.updateTopologyProfileTags([]*ddsnmp.ProfileMetrics{pm})
	cache.ingestTopologyProfileMetrics([]*ddsnmp.ProfileMetrics{pm})
	cache.ingestTopologyBGPPeers([]*ddsnmp.ProfileMetrics{pm})
	cache.finalizeTopologyCache()
	return cache
}

func (s *topologyScenario) deviceMetadata(dev *topologyScenarioDevice) map[string]ddsnmp.MetaTag {
	tags := map[string]ddsnmp.MetaTag{
		tagLldpLocChassisID:        {Value: dev.chassisMAC},
		tagLldpLocChassisIDSubtype: {Value: "4"},
		tagLldpLocSysName:          {Value: dev.name},
		tagLldpLocSysDesc:          {Value: topologyScenarioSysDescr},
	}
	if dev.capabilities != "" {
		tags[tagLldpLocSysCapSupported] = ddsnmp.MetaTag{Value: dev.capabilities}
		tags[tagLldpLocSysCapEnabled] = ddsnmp.MetaTag{Value: dev.capabilities}
	}
	if dev.routerID != "" {
		tags[tagOSPFRouterID] = ddsnmp.MetaTag{Value: dev.routerID}
	}
	if dev.chassisMAC != "" {
		tags[tagBridgeBaseAddress] = ddsnmp.MetaTag{Value: dev.chassisMAC}
	}
	return tags
}

func (s *topologyScenario) topologyMetricsForDevice(dev *topologyScenarioDevice) []ddsnmp.Metric {
	var metrics []ddsnmp.Metric
	for _, port := range dev.ports {
		metrics = append(metrics,
			topologyScenarioMetric(ddsnmp.KindIfName, topologyScenarioIfTags(port)),
			topologyScenarioMetric(ddsnmp.KindBridgePortIfIndex, map[string]string{
				tagBridgeBaseAddress: dev.chassisMAC,
				tagBridgeBasePort:    port.bridgePort,
				tagBridgeIfIndex:     strconv.Itoa(port.ifIndex),
			}),
			topologyScenarioMetric(ddsnmp.KindLldpLocPort, map[string]string{
				tagLldpLocPortNum:       topologyScenarioPortNum(port),
				tagLldpLocPortID:        port.name,
				tagLldpLocPortIDSubtype: "5",
				tagLldpLocPortDesc:      port.name,
			}),
		)
		if port.ip != "" && port.netmask != "" {
			metrics = append(metrics, topologyScenarioMetric(ddsnmp.KindIpIfIndex, map[string]string{
				tagTopoIfIndex: strconv.Itoa(port.ifIndex),
				tagTopoIPAddr:  port.ip,
				tagTopoIPMask:  port.netmask,
			}))
		}
	}
	for _, pair := range s.lldp {
		if pair.a.device == dev {
			metrics = append(metrics, topologyScenarioMetric(ddsnmp.KindLldpRem, topologyScenarioLLDPRemoteTags(pair.a, pair.b)))
		}
	}
	for _, pair := range s.cdp {
		if pair.a.device == dev {
			metrics = append(metrics, topologyScenarioMetric(ddsnmp.KindCdpCache, topologyScenarioCDPRemoteTags(pair.a, pair.b)))
		}
	}
	for _, pair := range s.stp {
		if pair.a.device == dev {
			metrics = append(metrics, topologyScenarioMetric(ddsnmp.KindStpPort, topologyScenarioSTPTags(pair.a, pair.b)))
		}
	}
	for _, pair := range s.ospf {
		if pair.a.device == dev {
			metrics = append(metrics, topologyScenarioMetric(ddsnmp.KindOSPFNeighbor, topologyScenarioOSPFTags(pair.a, pair.b)))
		}
	}
	for _, attachment := range s.fdbARP {
		if attachment.port.device == dev {
			metrics = append(metrics,
				topologyScenarioMetric(ddsnmp.KindFdbEntry, map[string]string{
					tagBridgeBaseAddress: dev.chassisMAC,
					tagFdbMac:            attachment.mac,
					tagFdbBridgePort:     attachment.port.bridgePort,
					tagFdbStatus:         "learned",
				}),
				topologyScenarioMetric(ddsnmp.KindArpEntry, map[string]string{
					tagArpIfIndex: strconv.Itoa(attachment.port.ifIndex),
					tagArpIfName:  attachment.port.name,
					tagArpIP:      attachment.ip,
					tagArpMac:     attachment.mac,
					tagArpState:   "reachable",
				}),
			)
		}
	}
	return metrics
}

func (s *topologyScenario) bgpRowsForDevice(dev *topologyScenarioDevice) []ddsnmp.BGPRow {
	var rows []ddsnmp.BGPRow
	for _, adjacency := range s.bgp {
		switch dev {
		case adjacency.a:
			rows = append(rows, topologyScenarioBGPRow(adjacency.a, adjacency.b, adjacency.routingInstance))
		case adjacency.b:
			rows = append(rows, topologyScenarioBGPRow(adjacency.b, adjacency.a, adjacency.routingInstance))
		}
	}
	return rows
}

func topologyScenarioMetric(kind ddsnmp.TopologyKind, tags map[string]string) ddsnmp.Metric {
	return ddsnmp.Metric{TopologyKind: kind, Tags: tags}
}

func topologyScenarioIfTags(port *topologyScenarioPort) map[string]string {
	return map[string]string{
		tagTopoIfIndex:  strconv.Itoa(port.ifIndex),
		tagTopoIfName:   port.name,
		tagTopoIfDescr:  port.name,
		tagTopoIfAlias:  "scenario " + port.name,
		tagTopoIfPhys:   port.mac,
		tagTopoIfType:   "6",
		tagTopoIfAdmin:  "1",
		tagTopoIfOper:   "1",
		tagTopoIfSpeed:  "1000000000",
		tagTopoIfDuplex: "full",
	}
}

func topologyScenarioLLDPRemoteTags(local, remote *topologyScenarioPort) map[string]string {
	return map[string]string{
		tagLldpLocPortNum:          topologyScenarioPortNum(local),
		tagLldpRemIndex:            "1",
		tagLldpRemChassisID:        remote.device.chassisMAC,
		tagLldpRemChassisIDSubtype: "4",
		tagLldpRemPortID:           remote.name,
		tagLldpRemPortIDSubtype:    "5",
		tagLldpRemPortDesc:         remote.name,
		tagLldpRemSysName:          remote.device.name,
		tagLldpRemSysDesc:          topologyScenarioSysDescr,
		tagLldpRemMgmtAddr:         remote.device.mgmtIP,
		tagLldpRemMgmtAddrSubtype:  "1",
		tagLldpRemSysCapSupported:  remote.device.capabilities,
		tagLldpRemSysCapEnabled:    remote.device.capabilities,
	}
}

func topologyScenarioCDPRemoteTags(local, remote *topologyScenarioPort) map[string]string {
	return map[string]string{
		tagCdpIfIndex:             strconv.Itoa(local.ifIndex),
		tagCdpIfName:              local.name,
		tagCdpDeviceIndex:         "1",
		tagCdpDeviceID:            remote.device.name,
		tagCdpAddressType:         "1",
		tagCdpAddress:             topologyScenarioIPv4Hex(remote.device.mgmtIP),
		tagCdpDevicePort:          remote.name,
		tagCdpVersion:             "synthetic",
		tagCdpPlatform:            "synthetic",
		tagCdpCaps:                remote.device.actorType,
		tagCdpSysName:             remote.device.name,
		tagCdpPrimaryMgmtAddrType: "1",
		tagCdpPrimaryMgmtAddr:     topologyScenarioIPv4Hex(remote.device.mgmtIP),
	}
}

func topologyScenarioSTPTags(local, designated *topologyScenarioPort) map[string]string {
	return map[string]string{
		tagBridgeBaseAddress:       local.device.chassisMAC,
		tagStpPort:                 local.bridgePort,
		tagStpPortState:            "forwarding",
		tagStpPortEnable:           "enabled",
		tagStpPortPathCost:         "4",
		tagStpPortDesignatedRoot:   designated.device.chassisMAC,
		tagStpPortDesignatedBridge: designated.device.chassisMAC,
		tagStpPortDesignatedPort:   designated.bridgePort,
	}
}

func topologyScenarioOSPFTags(local, remote *topologyScenarioPort) map[string]string {
	return map[string]string{
		tagOSPFNeighborRouterID:         remote.device.routerID,
		tagOSPFNeighborIP:               remote.ip,
		tagOSPFNeighborAddresslessIndex: "0",
		tagOSPFNeighborState:            "full",
	}
}

func topologyScenarioBGPRow(local, remote *topologyScenarioDevice, routingInstance string) ddsnmp.BGPRow {
	localIP, remoteIP := topologyScenarioBGPIPs(local, remote)
	return ddsnmp.BGPRow{
		Kind:         ddprofiledefinition.BGPRowKindPeer,
		StructuralID: strings.Join([]string{local.name, remote.name, routingInstance}, ":"),
		Identity: ddsnmp.BGPIdentity{
			RoutingInstance: routingInstance,
			Neighbor:        remoteIP,
			RemoteAS:        remote.localAS,
		},
		Descriptors: ddsnmp.BGPDescriptors{
			LocalAddress:    localIP,
			LocalAS:         local.localAS,
			LocalIdentifier: local.routerID,
			PeerIdentifier:  remote.routerID,
			PeerType:        "external",
			BGPVersion:      "4",
			Description:     "synthetic peer " + remote.name,
		},
		Admin: ddsnmp.BGPAdmin{
			Enabled: ddsnmp.BGPBool{Has: true, Value: true},
		},
		State: ddsnmp.BGPState{
			Has:   true,
			State: ddprofiledefinition.BGPPeerStateEstablished,
		},
		Connection: ddsnmp.BGPConnection{
			EstablishedUptime: ddsnmp.BGPInt64{Has: true, Value: 300},
		},
	}
}

func topologyScenarioBGPIPs(local, remote *topologyScenarioDevice) (string, string) {
	for _, localPort := range local.ports {
		if localPort.ip == "" {
			continue
		}
		for _, remotePort := range remote.ports {
			if remotePort.ip == "" {
				continue
			}
			if topologyScenarioSameSubnet(localPort, remotePort) {
				return localPort.ip, remotePort.ip
			}
		}
	}
	return local.mgmtIP, remote.mgmtIP
}

func topologyScenarioSameSubnet(a, b *topologyScenarioPort) bool {
	if a.netmask == "" || b.netmask == "" || a.netmask != b.netmask {
		return false
	}
	ip := net.ParseIP(a.ip)
	mask := net.ParseIP(a.netmask)
	remote := net.ParseIP(b.ip)
	if ip == nil || mask == nil || remote == nil {
		return false
	}
	return ip.Mask(net.IPMask(mask.To4())).Equal(remote.Mask(net.IPMask(mask.To4())))
}

func topologyScenarioPortNum(port *topologyScenarioPort) string {
	return strconv.Itoa(port.ifIndex)
}

func topologyScenarioCapabilities(actorType string) string {
	switch actorType {
	case "router":
		return "08"
	case "switch":
		return "20"
	default:
		return ""
	}
}

func topologyScenarioPortMAC(chassisMAC string, ifIndex int) string {
	mac := topologyutil.NormalizeMAC(chassisMAC)
	if mac == "" {
		return ""
	}
	// Keep synthetic interface MACs unique across devices; duplicate port MACs
	// collapse projected actors before semantic link assertions can see them.
	h := fnv.New64a()
	_, _ = h.Write([]byte(mac))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(strconv.Itoa(ifIndex)))
	sum := h.Sum64()
	return fmt.Sprintf("02:%02x:%02x:%02x:%02x:%02x",
		byte(sum>>32),
		byte(sum>>24),
		byte(sum>>16),
		byte(sum>>8),
		byte(sum),
	)
}

func topologyScenarioIPv4CIDR(cidr string) (string, string) {
	ip, network, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil || ip.To4() == nil {
		return "", ""
	}
	mask := net.IP(network.Mask).String()
	return ip.String(), mask
}

func topologyScenarioIPv4Hex(ip string) string {
	parsed := net.ParseIP(strings.TrimSpace(ip)).To4()
	if parsed == nil {
		return ""
	}
	return fmt.Sprintf("%02x%02x%02x%02x", parsed[0], parsed[1], parsed[2], parsed[3])
}
