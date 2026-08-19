//go:build unix

package apps_lookup

import (
	"github.com/netdata/netdata/go/plugins/pkg/netipc/service/internal/transportconfig"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/posix"
)

func clientConfigToTransport(config ClientConfig) posix.ClientConfig {
	return transportconfig.PosixClient(transportconfig.TypedConfig(config))
}

func serverConfigToTransport(config ServerConfig) posix.ServerConfig {
	return transportconfig.PosixServer(transportconfig.TypedConfig(config))
}
