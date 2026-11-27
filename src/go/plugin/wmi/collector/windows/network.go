// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import "strings"

// The byte totals decide whether an interface is charted and feed the
// aggregate bandwidth; the current bandwidth also feeds the speed variable.
const (
	bytesReceived = iota
	bytesSent
	currentBandwidth
)

// interfaceCounters are the raw counters of the Network Interface and Network
// Adapter counter sets that feed the native Windows Agent's per-interface
// charts.
var interfaceCounters = [...]rawCounter{
	bytesReceived:    {property: "BytesReceivedPersec", metric: "network_received_bytes", total: true},
	bytesSent:        {property: "BytesSentPersec", metric: "network_sent_bytes", total: true},
	currentBandwidth: {property: "CurrentBandwidth", metric: "network_speed_bits_per_second"},
	{property: "PacketsReceivedPersec", metric: "network_received_packets", total: true},
	{property: "PacketsSentPersec", metric: "network_sent_packets", total: true},
	{property: "PacketsReceivedErrors", metric: "network_inbound_errors", total: true},
	{property: "PacketsOutboundErrors", metric: "network_outbound_errors", total: true},
	{property: "PacketsReceivedDiscarded", metric: "network_inbound_discards", total: true},
	{property: "PacketsOutboundDiscarded", metric: "network_outbound_discards", total: true},
	{property: "OutputQueueLength", metric: "network_output_queue_length"},
	{property: "TCPActiveRSCConnections", metric: "network_rsc_connections"},
	{property: "TCPRSCCoalescedPacketsPersec", metric: "network_rsc_coalesced_packets", total: true},
	{property: "TCPRSCExceptionsPersec", metric: "network_rsc_exceptions", total: true},
	{property: "TCPRSCAveragePacketSize", metric: "network_rsc_average_packet_size_bytes"},
	{property: "OffloadedConnections", metric: "network_chimney_connections"},
	{property: "PacketsReceivedUnknown", metric: "network_unknown_protocol_packets", total: true},
}

var (
	queryNetworkInterfaces = counterQuery("Win32_PerfRawData_Tcpip_NetworkInterface", interfaceCounters[:], "Name")
	queryNetworkAdapters   = counterQuery("Win32_PerfRawData_Tcpip_NetworkAdapter", interfaceCounters[:], "Name")
)

// networkInterface holds one interface's counter values. The native Windows
// Agent calls the instances of the Network Interface counter set real and the
// Network Adapter instances missing from it virtual.
type networkInterface struct {
	device  string
	virtual bool
	values  [len(interfaceCounters)]sample
}

// readNetworkInterfaces keeps the interfaces that have carried traffic, as the
// native Windows Agent does: an interface whose counters show no byte since they
// started, such as an unused tunnel adapter, is not charted. The aggregate
// counts every interface.
func (o *observations) readNetworkInterfaces(rows []map[string]any) bool {
	names := make(map[string]bool, len(rows))
	var usable bool
	for _, row := range rows {
		nic, ok := readInterface(row)
		if !ok {
			continue
		}
		names[nic.device] = true
		received, sent := nic.values[bytesReceived], nic.values[bytesSent]
		if !received.valid && !sent.valid {
			continue
		}
		usable = true
		accumulate(&o.netTotalReceived, received)
		accumulate(&o.netTotalSent, sent)
		if nic.carriedTraffic() {
			o.interfaces = append(o.interfaces, nic)
		}
	}
	if len(rows) != 0 && !usable {
		return false
	}
	o.physicalInterfaces = names
	return true
}

// readNetworkAdapters adds the Network Adapter instances that this poll's
// Network Interface rows do not list, such as adapters without a TCP/IP
// binding. It runs only after readNetworkInterfaces named an interface.
func (o *observations) readNetworkAdapters(rows []map[string]any) bool {
	var usable bool
	for _, row := range rows {
		nic, ok := readInterface(row)
		if !ok || (!nic.values[bytesReceived].valid && !nic.values[bytesSent].valid) {
			continue
		}
		usable = true
		if o.physicalInterfaces[nic.device] || !nic.carriedTraffic() {
			continue
		}
		nic.virtual = true
		o.interfaces = append(o.interfaces, nic)
	}
	return len(rows) == 0 || usable
}

// readInterface reads a named instance; the counter sets' _Total is not one.
func readInterface(row map[string]any) (networkInterface, bool) {
	name, _ := row["name"].(string)
	if strings.TrimSpace(name) == "" || strings.EqualFold(name, "_Total") {
		return networkInterface{}, false
	}
	nic := networkInterface{
		device: name,
	}
	readCounters(row, interfaceCounters[:], nic.values[:])
	return nic, true
}

func (nic networkInterface) carriedTraffic() bool {
	return nic.values[bytesReceived].value != 0 || nic.values[bytesSent].value != 0
}

func accumulate(total *sample, s sample) {
	if s.valid {
		total.value += s.value
		total.valid = true
	}
}
