//go:build linux || (freebsd && (amd64 || 386))

package raw

import "testing"

func TestCallNoAllocsShm(t *testing.T) {
	assertCallsDoNotAllocate(t, "go_alloc_shm", testUnixShmServerConfig(), testUnixShmClientConfig())
}
