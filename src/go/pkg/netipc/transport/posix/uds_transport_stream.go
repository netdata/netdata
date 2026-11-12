//go:build darwin || freebsd

package posix

import "syscall"

// macOS has no AF_UNIX SOCK_SEQPACKET, and FreeBSD 15 SEQPACKET keeps record
// boundaries only where the sender sets MSG_EOR and can write part of a
// record. Both use a stream socket that carries the same packets, each
// delimited by the length its header declares (docs/level1-posix-uds.md,
// "Stream baseline").
const (
	sockType        = syscall.SOCK_STREAM
	streamTransport = true
	// staleDialNetwork is the net package's name for sockType.
	staleDialNetwork = "unix"
)

// kernelPacketLimit: a stream has no per-message size cap; the fixed cap
// keeps the first packet's buffer bounded and the chunking code shared.
func kernelPacketLimit(int) uint32 {
	return packetSizeMax
}
