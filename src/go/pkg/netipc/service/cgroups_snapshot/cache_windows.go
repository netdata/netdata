//go:build windows

package cgroups_snapshot

import (
	raw "github.com/netdata/netdata/go/plugins/pkg/netipc/service/raw"
)

// NewCache creates a new L3 cache. Does NOT connect.
func NewCache(runDir, serviceName string, config ClientConfig) *Cache {
	return &Cache{inner: raw.NewCache(runDir, serviceName, clientConfigToTransport(config))}
}
