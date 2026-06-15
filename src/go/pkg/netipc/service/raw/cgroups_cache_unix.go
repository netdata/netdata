//go:build unix

// L3: Client-side cgroups snapshot cache (POSIX).

package raw

import (
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/posix"
)

// NewCache creates a new L3 cache. Creates the underlying L2 client
// context. Does NOT connect. Does NOT require the server to be running.
// Cache starts empty (populated == false).
func NewCache(runDir, serviceName string, config posix.ClientConfig) *Cache {
	return newCache(NewSnapshotClient(runDir, serviceName, config))
}
