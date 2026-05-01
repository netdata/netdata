// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"context"
	"testing"

	gobgpapi "github.com/osrg/gobgp/v4/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_InitGoBGPDefaultsAddress(t *testing.T) {
	collr := New()
	collr.Backend = backendGoBGP
	collr.SocketPath = ""
	collr.Address = ""
	collr.newClient = func(cfg Config) (bgpClient, error) {
		assert.Equal(t, defaultAddressGoBGP, cfg.Address)
		return &mockClient{}, nil
	}

	require.NoError(t, collr.Init(context.Background()))
	assert.Equal(t, defaultAddressGoBGP, collr.Address)
	assert.Equal(t, "", collr.SocketPath)
}

func TestCollector_InitGoBGPInvalidAddress(t *testing.T) {
	collr := New()
	collr.Backend = backendGoBGP
	collr.SocketPath = ""
	collr.Address = "127.0.0.1"
	collr.newClient = func(Config) (bgpClient, error) { return &mockClient{}, nil }

	require.Error(t, collr.Init(context.Background()))
}

func TestGoBGPTableRequest(t *testing.T) {
	tableType, name := gobgpTableRequest(&gobgpFamilyRef{VRF: "default"})
	assert.Equal(t, gobgpapi.TableType_TABLE_TYPE_GLOBAL, tableType)
	assert.Equal(t, "", name)

	tableType, name = gobgpTableRequest(&gobgpFamilyRef{VRF: "blue"})
	assert.Equal(t, gobgpapi.TableType_TABLE_TYPE_VRF, tableType)
	assert.Equal(t, "blue", name)
}
