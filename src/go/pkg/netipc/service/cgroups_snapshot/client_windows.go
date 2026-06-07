//go:build windows

package cgroups_snapshot

import (
	windows "github.com/netdata/netdata/go/plugins/pkg/netipc/transport/windows"
)

func clientConfigToTransport(config ClientConfig) windows.ClientConfig {
	return windows.ClientConfig{
		SupportedProfiles:       config.SupportedProfiles,
		PreferredProfiles:       config.PreferredProfiles,
		MaxRequestBatchItems:    config.MaxRequestBatchItems,
		MaxResponsePayloadBytes: config.MaxResponsePayloadBytes,
		MaxResponseBatchItems:   typedResponseBatchItems(config.MaxRequestBatchItems),
		AuthToken:               config.AuthToken,
	}
}

func serverConfigToTransport(config ServerConfig) windows.ServerConfig {
	return windows.ServerConfig{
		SupportedProfiles:       config.SupportedProfiles,
		PreferredProfiles:       config.PreferredProfiles,
		MaxRequestBatchItems:    config.MaxRequestBatchItems,
		MaxResponsePayloadBytes: config.MaxResponsePayloadBytes,
		MaxResponseBatchItems:   typedResponseBatchItems(config.MaxRequestBatchItems),
		AuthToken:               config.AuthToken,
	}
}
