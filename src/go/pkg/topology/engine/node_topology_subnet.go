// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"fmt"
	"net/netip"
	"sort"
	"strconv"
)

var (
	pointToPointMaskIPv4 = netip.MustParseAddr("255.255.255.252")
	pointToPointMaskIPv6 = netip.MustParseAddr("ffff:ffff:ffff:ffff:ffff:ffff:ffff:fffe")
	loopbackMaskIPv4     = netip.MustParseAddr("255.255.255.255")
	loopbackMaskIPv6     = netip.MustParseAddr("ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff")
	loopbackAddrIPv4     = netip.MustParseAddr("127.0.0.1")
)

// SubNetwork mirrors Enlinkd SubNetwork behavior for node/IP membership.
type SubNetwork struct {
	network          netip.Addr
	netmask          netip.Addr
	nodeInterfaceMap map[int]map[netip.Addr]struct{}
}

// NewSubNetwork creates a subnet from one managed IP interface.
func NewSubNetwork(nodeID int, ip, netmask netip.Addr) (*SubNetwork, error) {
	if nodeID <= 0 {
		return nil, fmt.Errorf("node id is required")
	}
	if !ip.IsValid() {
		return nil, fmt.Errorf("ip is required")
	}
	if !netmask.IsValid() {
		return nil, fmt.Errorf("netmask is required")
	}
	network, ok := NetworkAddress(ip, netmask)
	if !ok {
		return nil, fmt.Errorf("cannot build network from ip %q and netmask %q", ip, netmask)
	}
	s := &SubNetwork{
		network:          network,
		netmask:          netmask,
		nodeInterfaceMap: map[int]map[netip.Addr]struct{}{},
	}
	s.nodeInterfaceMap[nodeID] = map[netip.Addr]struct{}{ip.Unmap(): {}}
	return s, nil
}

// Network returns the network address.
func (s *SubNetwork) Network() netip.Addr {
	if s == nil {
		return netip.Addr{}
	}
	return s.network
}

// Netmask returns the subnet mask.
func (s *SubNetwork) Netmask() netip.Addr {
	if s == nil {
		return netip.Addr{}
	}
	return s.netmask
}

// CIDR returns network/prefix format.
func (s *SubNetwork) CIDR() string {
	if s == nil || !s.network.IsValid() || !s.netmask.IsValid() {
		return ""
	}
	prefix, err := MaskToCIDRPrefix(s.netmask)
	if err != nil {
		return ""
	}
	return s.network.String() + "/" + strconv.Itoa(prefix)
}

// NetworkPrefix returns the CIDR prefix for the mask.
func (s *SubNetwork) NetworkPrefix() int {
	if s == nil {
		return 0
	}
	prefix, err := MaskToCIDRPrefix(s.netmask)
	if err != nil {
		return 0
	}
	return prefix
}

// IsIPv4Subnetwork reports if the subnet uses IPv4.
func (s *SubNetwork) IsIPv4Subnetwork() bool {
	return s != nil && s.network.IsValid() && s.network.Is4()
}

// NodeIDs returns sorted node IDs in the subnet.
func (s *SubNetwork) NodeIDs() []int {
	if s == nil || len(s.nodeInterfaceMap) == 0 {
		return nil
	}
	ids := make([]int, 0, len(s.nodeInterfaceMap))
	for nodeID := range s.nodeInterfaceMap {
		ids = append(ids, nodeID)
	}
	sort.Ints(ids)
	return ids
}

// Add adds one node/IP membership if the address is in range.
func (s *SubNetwork) Add(nodeID int, ip netip.Addr) bool {
	if s == nil || nodeID <= 0 || !ip.IsValid() || !s.IsInRange(ip) {
		return false
	}
	ip = ip.Unmap()
	if _, ok := s.nodeInterfaceMap[nodeID]; !ok {
		s.nodeInterfaceMap[nodeID] = map[netip.Addr]struct{}{}
	}
	if _, exists := s.nodeInterfaceMap[nodeID][ip]; exists {
		return false
	}
	s.nodeInterfaceMap[nodeID][ip] = struct{}{}
	return true
}

// Remove removes one node/IP membership.
func (s *SubNetwork) Remove(nodeID int, ip netip.Addr) bool {
	if s == nil || nodeID <= 0 || !ip.IsValid() {
		return false
	}
	ip = ip.Unmap()
	ips, ok := s.nodeInterfaceMap[nodeID]
	if !ok {
		return false
	}
	if _, exists := ips[ip]; !exists {
		return false
	}
	delete(ips, ip)
	if len(ips) == 0 {
		delete(s.nodeInterfaceMap, nodeID)
	}
	return true
}

// IsInRange reports if ip belongs to this subnet.
func (s *SubNetwork) IsInRange(ip netip.Addr) bool {
	if s == nil || !ip.IsValid() || !s.network.IsValid() || !s.netmask.IsValid() {
		return false
	}
	return InSameNetwork(ip.Unmap(), s.network, s.netmask)
}

// HasDuplicatedAddress reports true when the same address exists under multiple entries.
func (s *SubNetwork) HasDuplicatedAddress() bool {
	if s == nil {
		return false
	}
	seen := make(map[netip.Addr]struct{})
	for _, addresses := range s.nodeInterfaceMap {
		for addr := range addresses {
			if _, ok := seen[addr]; ok {
				return true
			}
			seen[addr] = struct{}{}
		}
	}
	return false
}

func (s *SubNetwork) clone() *SubNetwork {
	if s == nil {
		return nil
	}
	out := &SubNetwork{
		network:          s.network,
		netmask:          s.netmask,
		nodeInterfaceMap: make(map[int]map[netip.Addr]struct{}, len(s.nodeInterfaceMap)),
	}
	for nodeID, ips := range s.nodeInterfaceMap {
		copySet := make(map[netip.Addr]struct{}, len(ips))
		for ip := range ips {
			copySet[ip] = struct{}{}
		}
		out.nodeInterfaceMap[nodeID] = copySet
	}
	return out
}

func (s *SubNetwork) key() string {
	if s == nil {
		return ""
	}
	return subnetKey(s.network, s.netmask)
}

func subnetKey(network, netmask netip.Addr) string {
	if !network.IsValid() || !netmask.IsValid() {
		return ""
	}
	// Unmap IPv4-mapped IPv6 addresses so that ::ffff:10.0.0.0 and 10.0.0.0
	// produce the same key.
	network = network.Unmap()
	netmask = netmask.Unmap()
	return network.String() + keySep + netmask.String()
}

func sortedSubnetworkKeys(subnets map[string]*SubNetwork) []string {
	keys := make([]string, 0, len(subnets))
	for key := range subnets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func compareAddr(a, b netip.Addr) int {
	ab := addrBytes(a)
	bb := addrBytes(b)
	if len(ab) != len(bb) {
		if len(ab) < len(bb) {
			return -1
		}
		return 1
	}
	for i := range ab {
		if ab[i] < bb[i] {
			return -1
		}
		if ab[i] > bb[i] {
			return 1
		}
	}
	return 0
}

// IsPointToPointMask ports InetAddressUtils.isPointToPointMask().
func IsPointToPointMask(mask netip.Addr) bool {
	mask = mask.Unmap()
	return mask == pointToPointMaskIPv4 || mask == pointToPointMaskIPv6
}

// IsLoopbackMask ports InetAddressUtils.isLoopbackMask().
func IsLoopbackMask(mask netip.Addr) bool {
	mask = mask.Unmap()
	return mask == loopbackMaskIPv4 || mask == loopbackMaskIPv6
}

// InSameNetwork ports InetAddressUtils.inSameNetwork().
func InSameNetwork(addr1, addr2, mask netip.Addr) bool {
	addr1 = addr1.Unmap()
	addr2 = addr2.Unmap()
	mask = mask.Unmap()
	if !addr1.IsValid() || !addr2.IsValid() || !mask.IsValid() {
		return false
	}
	if addr1.Is4() != addr2.Is4() || addr1.Is4() != mask.Is4() {
		return false
	}

	ab := addrBytes(addr1)
	bb := addrBytes(addr2)
	mb := addrBytes(mask)
	if len(ab) != len(bb) || len(ab) != len(mb) {
		return false
	}
	for i := range ab {
		if (ab[i] & mb[i]) != (bb[i] & mb[i]) {
			return false
		}
	}
	return true
}

// NetworkAddress returns ip&mask for matching IP families.
func NetworkAddress(ip, mask netip.Addr) (netip.Addr, bool) {
	ip = ip.Unmap()
	mask = mask.Unmap()
	if !ip.IsValid() || !mask.IsValid() || ip.Is4() != mask.Is4() {
		return netip.Addr{}, false
	}
	ib := addrBytes(ip)
	mb := addrBytes(mask)
	if len(ib) != len(mb) {
		return netip.Addr{}, false
	}
	out := make([]byte, len(ib))
	for i := range ib {
		out[i] = ib[i] & mb[i]
	}
	addr, ok := netip.AddrFromSlice(out)
	if !ok {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

// MaskToCIDRPrefix ports InetAddressUtils.convertInetAddressMaskToCidr().
func MaskToCIDRPrefix(mask netip.Addr) (int, error) {
	mask = mask.Unmap()
	if !mask.IsValid() {
		return 0, fmt.Errorf("invalid mask")
	}
	foundZero := false
	cidr := 0
	for _, value := range addrBytes(mask) {
		k := int(value)
		if foundZero && k != 0 {
			return 0, fmt.Errorf("invalid mask %q", mask)
		}
		switch k {
		case 255:
			cidr += 8
		case 254:
			cidr += 7
			foundZero = true
		case 252:
			cidr += 6
			foundZero = true
		case 248:
			cidr += 5
			foundZero = true
		case 240:
			cidr += 4
			foundZero = true
		case 224:
			cidr += 3
			foundZero = true
		case 192:
			cidr += 2
			foundZero = true
		case 128:
			cidr += 1
			foundZero = true
		case 0:
			foundZero = true
		default:
			return 0, fmt.Errorf("invalid mask %q", mask)
		}
	}
	return cidr, nil
}

func addrBytes(addr netip.Addr) []byte {
	if !addr.IsValid() {
		return nil
	}
	if addr.Is4() {
		a := addr.As4()
		return a[:]
	}
	a := addr.As16()
	return a[:]
}
