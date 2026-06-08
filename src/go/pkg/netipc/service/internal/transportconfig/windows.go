//go:build windows

package transportconfig

import "github.com/netdata/netdata/go/plugins/pkg/netipc/transport/windows"

func WindowsClient(config TypedConfig) windows.ClientConfig {
	return windows.ClientConfig{
		SupportedProfiles:       config.SupportedProfiles,
		PreferredProfiles:       config.PreferredProfiles,
		MaxRequestBatchItems:    config.MaxRequestBatchItems,
		MaxResponsePayloadBytes: config.MaxResponsePayloadBytes,
		MaxResponseBatchItems:   responseBatchItems(config),
		AuthToken:               config.AuthToken,
	}
}

func WindowsServer(config TypedConfig) windows.ServerConfig {
	return windows.ServerConfig{
		SupportedProfiles:       config.SupportedProfiles,
		PreferredProfiles:       config.PreferredProfiles,
		MaxRequestBatchItems:    config.MaxRequestBatchItems,
		MaxResponsePayloadBytes: config.MaxResponsePayloadBytes,
		MaxResponseBatchItems:   responseBatchItems(config),
		AuthToken:               config.AuthToken,
	}
}
