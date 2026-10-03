package libbpfloader

// DNSFlowRecord is one completed DNS query/response pair from the 20-second
// per-query ring maintained by the C BPF runtime.
type DNSFlowRecord struct {
	TimestampUs uint64
	LatencyUs   uint64
	ServerIP    [4]uint32
	ClientIP    [4]uint32
	Domain      string
	ClientPort  uint16
	QueryType   uint16
	RCode       uint16
	Protocol    uint8
	IPVersion   uint8
	TimedOut    bool
}
