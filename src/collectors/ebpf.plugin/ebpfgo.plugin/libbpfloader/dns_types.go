package libbpfloader

// DNSSnapshot holds per-cycle DNS event counters drained from the BPF ring
// buffer (buffer/arena flavors) or the raw AF_PACKET socket (base flavor).
// Fields are per-cycle counts, not cumulative monotonic counters; the C layer
// resets them on every Snapshot call.
type DNSSnapshot struct {
	QueriesUDPv4   uint64
	QueriesUDPv6   uint64
	QueriesTCPv4   uint64
	QueriesTCPv6   uint64
	ResponsesUDPv4 uint64
	ResponsesUDPv6 uint64
	ResponsesTCPv4 uint64
	ResponsesTCPv6 uint64
}
