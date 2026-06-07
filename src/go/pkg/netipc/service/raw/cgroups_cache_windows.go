//go:build windows

// L3: Client-side cgroups snapshot cache (Windows).
//
// Identical cache logic as the POSIX version. Uses Windows Client.
//
// Pure Go — no cgo. Works with CGO_ENABLED=0.

package raw

import (
	windows "github.com/netdata/netdata/go/plugins/pkg/netipc/transport/windows"
)

// NewCache creates a new L3 cache.
func NewCache(runDir, serviceName string, config windows.ClientConfig) *Cache {
	return newCache(NewSnapshotClient(runDir, serviceName, config))
}
