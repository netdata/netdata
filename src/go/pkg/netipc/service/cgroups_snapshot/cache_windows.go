//go:build windows

package cgroups_snapshot

import (
	"github.com/netdata/netdata/go/plugins/pkg/netipc/service/internal/transportconfig"
	raw "github.com/netdata/netdata/go/plugins/pkg/netipc/service/raw"
)

// NewCache creates a new L3 cache. Does NOT connect.
func NewCache(runDir, serviceName string, config ClientConfig) *Cache {
	inner := raw.NewCache(runDir, serviceName, clientConfigToTransport(config))
	inner.SetCallTimeout(transportconfig.TypedConfig(config).CallTimeoutMs)
	return &Cache{inner: inner}
}
