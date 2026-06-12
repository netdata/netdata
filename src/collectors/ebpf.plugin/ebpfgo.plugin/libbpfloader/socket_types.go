package libbpfloader

// SocketSnapshot holds the raw monotonic counters read from the socket BPF
// maps in a single collection cycle.  Field order matches enum ebpf_socket_idx
// from ebpf_socket.h (keys 0-17 for tbl_global_sock) plus the two inbound
// connection counters aggregated from tbl_lports.
type SocketSnapshot struct {
	CallsTCPSendmsg       uint64 // key  0
	ErrorTCPSendmsg       uint64 // key  1
	BytesTCPSendmsg       uint64 // key  2
	CallsTCPCleanupRbuf   uint64 // key  3
	ErrorTCPCleanupRbuf   uint64 // key  4
	BytesTCPCleanupRbuf   uint64 // key  5
	CallsTCPClose         uint64 // key  6
	CallsUDPRecvmsg       uint64 // key  7
	ErrorUDPRecvmsg       uint64 // key  8
	BytesUDPRecvmsg       uint64 // key  9
	CallsUDPSendmsg       uint64 // key 10
	ErrorUDPSendmsg       uint64 // key 11
	BytesUDPSendmsg       uint64 // key 12
	TCPRetransmit         uint64 // key 13
	CallsTCPConnectIPv4   uint64 // key 14
	ErrorTCPConnectIPv4   uint64 // key 15
	CallsTCPConnectIPv6   uint64 // key 16
	ErrorTCPConnectIPv6   uint64 // key 17
	InboundConnTCP        uint64 // tbl_lports: TCP listening ports counter sum
	InboundConnUDP        uint64 // tbl_lports: UDP listening ports counter sum
}
