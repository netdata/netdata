//go:build unix && !darwin && !freebsd

package posix

import "syscall"

const (
	sockType        = syscall.SOCK_SEQPACKET
	streamTransport = false
	// staleDialNetwork is the net package's name for sockType.
	staleDialNetwork = "unixpacket"
	// sndbufOverhead is what Linux keeps from SO_SNDBUF for a SEQPACKET
	// datagram: it accepts SO_SNDBUF - 32.
	sndbufOverhead = 32
)

// kernelPacketLimit is the largest datagram the kernel accepts. Linux
// rejects a datagram larger than SO_SNDBUF minus 32 bytes of accounting
// overhead with EMSGSIZE; very large buffers also hit ENOBUFS near 1 MiB,
// hence the fixed cap in packetSizeLimit.
func kernelPacketLimit(fd int) uint32 {
	limit := defaultPacketSizeFallback
	val, err := syscall.GetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_SNDBUF)
	if err == nil && val > sndbufOverhead {
		limit = uint32(val - sndbufOverhead) // #nosec G115 -- val is a positive int from the kernel.
	}
	return limit
}
