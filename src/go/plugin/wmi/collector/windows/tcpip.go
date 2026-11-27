// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import "strings"

// ipVersion indexes the counter sets that Windows keeps per IP version.
type ipVersion int

const (
	ipv4 ipVersion = iota
	ipv6
	ipVersions
)

// protocolCounter is a raw counter of the TCPv4 and TCPv6, or UDPv4 and UDPv6,
// counter sets: a TCP or UDP MIB-II counter of one IP version. The native
// Windows Agent charts the segments and datagrams per IP version and the
// segments of both versions summed. The other counters go to the charts of the
// Netdata Agent on Linux: TCP for both versions summed, as Linux counts TCP,
// and UDP per version.
type protocolCounter struct {
	property string
	metric   string
	// total marks a count since the counter started, whose rate Netdata charts.
	total bool
	// perVersion selects one series per IP version (ipv4_ or ipv6_ before the
	// metric name); summed selects one for both versions (the metric name).
	perVersion bool
	summed     bool
}

var tcpCounters = [...]protocolCounter{
	{property: "SegmentsReceivedPersec", metric: "tcp_segments_received", total: true, perVersion: true, summed: true},
	{property: "SegmentsSentPersec", metric: "tcp_segments_sent", total: true, perVersion: true, summed: true},
	{property: "ConnectionsEstablished", metric: "tcp_established_connections", summed: true},
	{property: "ConnectionsActive", metric: "tcp_active_opens", total: true, summed: true},
	{property: "ConnectionsPassive", metric: "tcp_passive_opens", total: true, summed: true},
	{property: "ConnectionsReset", metric: "tcp_established_resets", total: true, summed: true},
	{property: "ConnectionFailures", metric: "tcp_failed_connection_attempts", total: true, summed: true},
	{property: "SegmentsRetransmittedPersec", metric: "tcp_retransmitted_segments", total: true, summed: true},
}

var udpCounters = [...]protocolCounter{
	{property: "DatagramsReceivedPersec", metric: "udp_datagrams_received", total: true, perVersion: true},
	{property: "DatagramsSentPersec", metric: "udp_datagrams_sent", total: true, perVersion: true},
	{property: "DatagramsReceivedErrors", metric: "udp_receive_errors", total: true, perVersion: true},
	{property: "DatagramsNoPortPersec", metric: "udp_no_port_datagrams", total: true, perVersion: true},
}

var ipVersionPrefixes = [ipVersions]string{
	ipv4: "ipv4_",
	ipv6: "ipv6_",
}

var (
	queryTCPv4 = protocolQuery("Win32_PerfRawData_Tcpip_TCPv4", tcpCounters[:])
	queryTCPv6 = protocolQuery("Win32_PerfRawData_Tcpip_TCPv6", tcpCounters[:])
	queryUDPv4 = protocolQuery("Win32_PerfRawData_Tcpip_UDPv4", udpCounters[:])
	queryUDPv6 = protocolQuery("Win32_PerfRawData_Tcpip_UDPv6", udpCounters[:])
)

func protocolQuery(class string, counters []protocolCounter) string {
	properties := make([]string, 0, len(counters))
	for _, counter := range counters {
		properties = append(properties, counter.property)
	}
	return "SELECT " + strings.Join(properties, ", ") + " FROM " + class
}

func (o *observations) readTCP(version ipVersion) func([]map[string]any) bool {
	return func(rows []map[string]any) bool {
		return readProtocol(rows, tcpCounters[:], o.tcp[version][:])
	}
}

func (o *observations) readUDP(version ipVersion) func([]map[string]any) bool {
	return func(rows []map[string]any) bool {
		return readProtocol(rows, udpCounters[:], o.udp[version][:])
	}
}

// readProtocol reads the one row of a TCP or UDP counter set.
func readProtocol(rows []map[string]any, counters []protocolCounter, values []sample) bool {
	if len(rows) != 1 {
		return false
	}
	var usable bool
	for i, counter := range counters {
		values[i] = readUnsigned(rows[0][strings.ToLower(counter.property)])
		usable = usable || values[i].valid
	}
	return usable
}

// bothVersions adds the IPv4 and IPv6 values of a counter, as the native
// Windows Agent adds its TCP segments. Without one of them there is no sum: a
// sum of one version would jump when the other returns.
//
// The counters are 32 bits wide, so the sum wraps at 32 bits too: it is the
// total of both versions modulo 2^32, which the Agent unwraps like any 32-bit
// counter. A plain sum can exceed 32 bits, after which the Agent counts each
// wrap of either version as an interval of 0.
func bothVersions(values [ipVersions]sample) sample {
	if !values[ipv4].valid || !values[ipv6].valid {
		return sample{}
	}
	return sample{
		value: uint64(uint32(values[ipv4].value + values[ipv6].value)),
		valid: true,
	}
}
