// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"encoding/hex"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"
)

func adjacencyKey(adj Adjacency) string {
	return strings.Join([]string{adj.Protocol, adj.SourceID, adj.SourcePort, adj.TargetID, adj.TargetPort}, "|")
}

func attachmentKey(attachment Attachment) string {
	vlanID := ""
	if len(attachment.Labels) > 0 {
		vlanID = strings.TrimSpace(attachment.Labels["vlan_id"])
		if vlanID == "" {
			vlanID = strings.TrimSpace(attachment.Labels["vlan"])
		}
	}
	return strings.Join([]string{
		attachment.DeviceID,
		strconv.Itoa(attachment.IfIndex),
		attachment.EndpointID,
		attachment.Method,
		strings.ToLower(vlanID),
	}, "|")
}

func ifaceKey(iface Interface) string {
	return fmt.Sprintf("%s|%d|%s", iface.DeviceID, iface.IfIndex, iface.IfName)
}

func deviceIfIndexKey(deviceID string, ifIndex int) string {
	return fmt.Sprintf("%s|%d", deviceID, ifIndex)
}

func deriveBridgeDomainFromIfIndex(deviceID string, ifIndex int) string {
	return fmt.Sprintf("bridge-domain:%s:if:%d", deviceID, ifIndex)
}

func deriveBridgeDomainFromBridgePort(deviceID, bridgePort string) string {
	return fmt.Sprintf("bridge-domain:%s:bp:%s", deviceID, bridgePort)
}

func attachmentDomain(attachment Attachment) string {
	if len(attachment.Labels) == 0 {
		return ""
	}
	return strings.TrimSpace(attachment.Labels["bridge_domain"])
}

func canonicalProtocol(protocol string) string {
	protocol = strings.TrimSpace(strings.ToLower(protocol))
	if protocol == "" {
		return "arp"
	}
	return protocol
}

func canonicalAddrType(addrType, ip string) string {
	addrType = strings.TrimSpace(strings.ToLower(addrType))
	if ipAddr := parseAddr(ip); ipAddr.IsValid() {
		if ipAddr.Is4() {
			return "ipv4"
		}
		return "ipv6"
	}
	if addrType == "" {
		return ""
	}
	return addrType
}

func sortedAddrValues(in map[string]netip.Addr) []netip.Addr {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]netip.Addr, 0, len(keys))
	for _, key := range keys {
		if addr, ok := in[key]; ok && addr.IsValid() {
			out = append(out, addr)
		}
	}
	return out
}

func setToCSV(in map[string]struct{}) string {
	if len(in) == 0 {
		return ""
	}
	out := make([]string, 0, len(in))
	for value := range in {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func csvToTopologySet(value string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, token := range strings.Split(strings.TrimSpace(value), ",") {
		token = strings.TrimSpace(strings.ToLower(token))
		if token == "" {
			continue
		}
		out[token] = struct{}{}
	}
	return out
}

func observationProtocolsUsed(obs L2Observation) map[string]struct{} {
	out := make(map[string]struct{}, 6)
	if len(obs.LLDPRemotes) > 0 {
		out["lldp"] = struct{}{}
	}
	if len(obs.CDPRemotes) > 0 {
		out["cdp"] = struct{}{}
	}
	if len(obs.BridgePorts) > 0 {
		out["bridge"] = struct{}{}
	}
	if len(obs.FDBEntries) > 0 {
		out["fdb"] = struct{}{}
	}
	if len(obs.STPPorts) > 0 {
		out["stp"] = struct{}{}
	}
	if len(obs.ARPNDEntries) > 0 {
		out["arp"] = struct{}{}
	}
	return out
}

func pruneEmptyLabels(labels map[string]string) {
	for key, value := range labels {
		if strings.TrimSpace(value) == "" {
			delete(labels, key)
		}
	}
}

func deriveRemoteDeviceID(hostname, chassisID, mgmtIP, fallback string) string {
	if host := canonicalHost(hostname); host != "" {
		return host
	}
	if ch := canonicalToken(chassisID); ch != "" {
		return "chassis-" + ch
	}
	if ip := canonicalIP(mgmtIP); ip != "" {
		return "ip-" + strings.ReplaceAll(ip, ":", "-")
	}
	if fb := canonicalHost(fallback); fb != "" {
		return fb
	}
	return "discovered-unknown"
}

func canonicalHost(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	v = strings.TrimSuffix(v, ".")
	return v
}

func normalizeLLDPPortIDForMatch(portID, subtype string) string {
	portID = strings.TrimSpace(portID)
	if portID == "" {
		return ""
	}

	switch normalizeLLDPPortSubtypeForMatch(subtype) {
	case "mac":
		if mac := canonicalLLDPMACToken(portID); mac != "" {
			return mac
		}
	case "network":
		if ip := canonicalIP(portID); ip != "" {
			return ip
		}
	}

	return portID
}

func normalizeLLDPPortSubtypeForMatch(subtype string) string {
	switch strings.ToLower(strings.TrimSpace(subtype)) {
	case "3", "macaddress":
		return "mac"
	case "4", "networkaddress":
		return "network"
	default:
		return strings.ToLower(strings.TrimSpace(subtype))
	}
}

func normalizeLLDPChassisForMatch(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if ip := canonicalIP(v); ip != "" {
		return ip
	}
	if mac := canonicalLLDPMACToken(v); mac != "" {
		return mac
	}
	return v
}

func canonicalLLDPMACToken(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return ""
	}

	clean := strings.TrimPrefix(v, "0x")
	clean = strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(clean)
	if len(clean) != 12 {
		return ""
	}
	if _, err := hex.DecodeString(clean); err != nil {
		return ""
	}
	return clean
}

func canonicalToken(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	v = strings.ReplaceAll(v, ":", "")
	v = strings.ReplaceAll(v, "-", "")
	v = strings.ReplaceAll(v, ".", "")
	v = strings.ReplaceAll(v, " ", "")
	return v
}

func canonicalBridgeAddr(value, fallback string) string {
	if mac := normalizeMAC(value); mac != "" {
		return mac
	}
	if mac := normalizeMAC(fallback); mac != "" {
		return mac
	}
	return ""
}

func primaryL2MACIdentity(chassisID, baseBridgeAddress string) string {
	for _, candidate := range []string{chassisID, baseBridgeAddress} {
		if mac := normalizeMAC(candidate); mac != "" && mac != "00:00:00:00:00:00" {
			return mac
		}
	}
	return ""
}

func canonicalIP(v string) string {
	if ip := parseAddr(v); ip.IsValid() {
		return ip.String()
	}
	if ip := parseAddr(decodeHexIP(v)); ip.IsValid() {
		return ip.String()
	}
	return ""
}

func parseAddr(v string) netip.Addr {
	addr, err := netip.ParseAddr(strings.TrimSpace(v))
	if err != nil {
		return netip.Addr{}
	}
	return addr.Unmap()
}

func decodeHexIP(v string) string {
	bs := decodeHexBytes(v)
	if len(bs) == 4 {
		addr, ok := netip.AddrFromSlice(bs)
		if ok {
			return addr.Unmap().String()
		}
	}
	if len(bs) == 16 {
		addr, ok := netip.AddrFromSlice(bs)
		if ok {
			return addr.String()
		}
	}
	return ""
}

func decodeHexBytes(v string) []byte {
	clean := strings.ToLower(strings.TrimSpace(v))
	clean = strings.TrimPrefix(clean, "0x")
	if clean == "" {
		return nil
	}

	if strings.ContainsAny(clean, ":-. \t") {
		parts := strings.FieldsFunc(clean, func(r rune) bool {
			return r == ':' || r == '-' || r == '.' || r == ' ' || r == '\t'
		})
		if len(parts) == 0 {
			return nil
		}

		out := make([]byte, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if len(part) > 2 {
				return nil
			}
			if len(part) == 1 {
				part = "0" + part
			}
			b, err := hex.DecodeString(part)
			if err != nil || len(b) != 1 {
				return nil
			}
			out = append(out, b[0])
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}

	if len(clean)%2 == 1 {
		clean = "0" + clean
	}
	bs, err := hex.DecodeString(clean)
	if err != nil {
		return nil
	}
	return bs
}

func normalizeMAC(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}

	if bs := parseDottedDecimalBytes(v); len(bs) == 6 {
		return formatMAC(bs)
	}
	if bs := decodeHexBytes(v); len(bs) == 6 {
		return formatMAC(bs)
	}
	return ""
}

func parseDottedDecimalBytes(v string) []byte {
	parts := strings.Split(strings.TrimSpace(v), ".")
	if len(parts) != 6 {
		return nil
	}
	out := make([]byte, 0, 6)
	for _, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 0 || n > 255 {
			return nil
		}
		out = append(out, byte(n))
	}
	return out
}

func formatMAC(bs []byte) string {
	if len(bs) != 6 {
		return ""
	}
	parts := make([]string, 0, 6)
	for _, b := range bs {
		parts = append(parts, fmt.Sprintf("%02x", b))
	}
	return strings.Join(parts, ":")
}
