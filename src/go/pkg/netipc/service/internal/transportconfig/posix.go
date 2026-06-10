//go:build unix

package transportconfig

import "github.com/netdata/netdata/go/plugins/pkg/netipc/transport/posix"

func PosixClient(config TypedConfig) posix.ClientConfig {
	return posix.ClientConfig{
		SupportedProfiles:       config.SupportedProfiles,
		PreferredProfiles:       config.PreferredProfiles,
		MaxRequestBatchItems:    config.MaxRequestBatchItems,
		MaxResponsePayloadBytes: config.MaxResponsePayloadBytes,
		MaxResponseBatchItems:   responseBatchItems(config),
		AuthToken:               config.AuthToken,
	}
}

func PosixServer(config TypedConfig) posix.ServerConfig {
	return posix.ServerConfig{
		SupportedProfiles:       config.SupportedProfiles,
		PreferredProfiles:       config.PreferredProfiles,
		MaxRequestBatchItems:    config.MaxRequestBatchItems,
		MaxResponsePayloadBytes: config.MaxResponsePayloadBytes,
		MaxResponseBatchItems:   responseBatchItems(config),
		AuthToken:               config.AuthToken,
	}
}
