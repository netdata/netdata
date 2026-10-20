//go:build windows

package cgroups_snapshot

import (
	"github.com/netdata/netdata/go/plugins/pkg/netipc/service/internal/transportconfig"
	windows "github.com/netdata/netdata/go/plugins/pkg/netipc/transport/windows"
)

func clientConfigToTransport(config ClientConfig) windows.ClientConfig {
	return transportconfig.WindowsClient(transportconfig.TypedConfig(config))
}

func serverConfigToTransport(config ServerConfig) windows.ServerConfig {
	return transportconfig.WindowsServer(transportconfig.TypedConfig(config))
}
