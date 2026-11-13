//go:build unix && !linux && !(freebsd && (amd64 || 386))

package posix

import "github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"

// platformProfiles limits the handshake to the baseline profile: POSIX SHM
// needs a futex-style wait (Linux futex, FreeBSD _umtx_op), and Go on
// FreeBSD provides it only on amd64 and 386.
const platformProfiles = protocol.ProfileBaseline

// ShmContext is a handle to a shared memory region. Without SHM support no
// context is ever created.
type ShmContext struct {
	role ShmRole
	fd   int

	SpinTries uint32
}

// Role returns the context role.
func (c *ShmContext) Role() ShmRole { return c.role }

// Fd returns the file descriptor.
func (c *ShmContext) Fd() int { return c.fd }

// ShmServerCreate reports that SHM is not supported here.
func ShmServerCreate(runDir, serviceName string, sessionID uint64, reqCapacity, respCapacity uint32) (*ShmContext, error) {
	return nil, ErrShmUnsupported
}

// ShmClientAttach reports that SHM is not supported here.
func ShmClientAttach(runDir, serviceName string, sessionID uint64) (*ShmContext, error) {
	return nil, ErrShmUnsupported
}

// ShmDestroy does nothing: no region exists.
func (c *ShmContext) ShmDestroy() {}

// ShmClose does nothing: no region exists.
func (c *ShmContext) ShmClose() {}

// ShmSend reports that SHM is not supported here.
func (c *ShmContext) ShmSend(msg []byte) error { return ErrShmUnsupported }

// ShmReceive reports that SHM is not supported here.
func (c *ShmContext) ShmReceive(buf []byte, timeoutMs uint32) (int, error) {
	return 0, ErrShmUnsupported
}

// ShmCleanupStale does nothing: no regions are ever created here.
func ShmCleanupStale(runDir, serviceName string) {}
