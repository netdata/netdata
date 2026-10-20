//go:build windows

package transportconfig

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

func TestWindowsConfigMapping(t *testing.T) {
	config := TypedConfig{
		SupportedProfiles:             protocol.ProfileBaseline,
		PreferredProfiles:             protocol.ProfileBaseline,
		MaxRequestPayloadBytes:        101,
		MaxRequestBatchItems:          7,
		MaxResponsePayloadBytes:       202,
		CallTimeoutMs:                 303,
		AuthToken:                     404,
		MaxLogicalLookupItems:         505,
		MaxLogicalLookupSubcalls:      606,
		MaxLogicalLookupResponseBytes: 707,
	}

	client := WindowsClient(config)
	if client.SupportedProfiles != config.SupportedProfiles ||
		client.PreferredProfiles != config.PreferredProfiles ||
		client.MaxRequestPayloadBytes != config.MaxRequestPayloadBytes ||
		client.MaxRequestBatchItems != config.MaxRequestBatchItems ||
		client.MaxResponsePayloadBytes != config.MaxResponsePayloadBytes ||
		client.MaxResponseBatchItems != config.MaxRequestBatchItems ||
		client.AuthToken != config.AuthToken {
		t.Fatalf("windows client config mismatch: %+v", client)
	}

	server := WindowsServer(config)
	if server.SupportedProfiles != config.SupportedProfiles ||
		server.PreferredProfiles != config.PreferredProfiles ||
		server.MaxRequestPayloadBytes != config.MaxRequestPayloadBytes ||
		server.MaxRequestBatchItems != config.MaxRequestBatchItems ||
		server.MaxResponsePayloadBytes != config.MaxResponsePayloadBytes ||
		server.MaxResponseBatchItems != config.MaxRequestBatchItems ||
		server.AuthToken != config.AuthToken {
		t.Fatalf("windows server config mismatch: %+v", server)
	}
}
