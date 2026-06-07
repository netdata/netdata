//go:build unix

package cgroups_lookup

import (
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/posix"
)

func clientConfigToTransport(config ClientConfig) posix.ClientConfig {
	return posix.ClientConfig{
		SupportedProfiles:       config.SupportedProfiles,
		PreferredProfiles:       config.PreferredProfiles,
		MaxRequestBatchItems:    config.MaxRequestBatchItems,
		MaxResponsePayloadBytes: config.MaxResponsePayloadBytes,
		MaxResponseBatchItems:   typedResponseBatchItems(config.MaxRequestBatchItems),
		AuthToken:               config.AuthToken,
	}
}

func serverConfigToTransport(config ServerConfig) posix.ServerConfig {
	return posix.ServerConfig{
		SupportedProfiles:       config.SupportedProfiles,
		PreferredProfiles:       config.PreferredProfiles,
		MaxRequestBatchItems:    config.MaxRequestBatchItems,
		MaxResponsePayloadBytes: config.MaxResponsePayloadBytes,
		MaxResponseBatchItems:   typedResponseBatchItems(config.MaxRequestBatchItems),
		AuthToken:               config.AuthToken,
	}
}
