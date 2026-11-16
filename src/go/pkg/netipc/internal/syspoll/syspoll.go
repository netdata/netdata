//go:build unix

// Package syspoll waits for readiness on one file descriptor using the raw
// poll family of system calls, so NetIPC stays pure Go (no cgo, no x/sys).
package syspoll

// Poll event bits from <poll.h>; the values are identical on Linux, FreeBSD
// and macOS.
const (
	In   int16 = 0x0001
	Out  int16 = 0x0004
	Err  int16 = 0x0008
	Hup  int16 = 0x0010
	Nval int16 = 0x0020
)

// PollFd matches struct pollfd from <poll.h>.
type PollFd struct {
	Fd      int32
	Events  int16
	Revents int16
}
