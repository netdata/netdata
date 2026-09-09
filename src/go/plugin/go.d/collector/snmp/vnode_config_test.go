// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestNamedVnodeClientInitializationDoesNotLogCredentials(t *testing.T) {
	c := New(ddsnmp.NewDeviceStore())
	c.Hostname = "192.0.2.1"
	c.Community = "fixture-collector-secret"
	c.Vnode = "router"
	var logs bytes.Buffer
	c.Logger = logger.NewWithWriter(&logs)
	_, err := c.initSNMPClient()
	require.NoError(t, err)
	require.NotContains(t, logs.String(), c.Community)
	c.Options.Version = "3"
	c.User.Name = "fixture-user"
	c.User.ContextName = "routing-context"
	c.User.AuthKey = "fixture-auth-secret"
	c.User.PrivKey = "fixture-priv-secret"
	logs.Reset()
	_, err = c.initSNMPClient()
	require.NoError(t, err)
	require.Contains(t, logs.String(), `context=\"routing-context\"`)
	require.NotContains(t, logs.String(), c.Community)
	require.NotContains(t, logs.String(), c.User.AuthKey)
	require.NotContains(t, logs.String(), c.User.PrivKey)
}

func TestNamedVnodeIgnoresUnusedLocalGUID(t *testing.T) {
	c := New(ddsnmp.NewDeviceStore())
	c.Hostname = "192.0.2.1"
	c.Vnode = "central"
	c.LocalVnode.GUID = "invalid-unused-guid"
	require.NoError(t, c.Init(context.Background()))
	c.Cleanup(context.Background())
	c.Vnode = ""
	require.ErrorContains(t, c.Init(context.Background()), "invalid Vnode GUID")
}

func TestVnodeConfigurationRoundTrip(t *testing.T) {
	for _, input := range []string{
		`{"hostname":"device","vnode":"router"}`,
		`{"hostname":"device","local_vnode":{"hostname":"local","labels":{"site":"test"}}}`,
		`{"hostname":"device","vnode":{"hostname":"inline","guid":"6e17cd2c-0518-4e94-965b-d25673decb21","labels":{"site":"test"}}}`,
		`{"hostname":"device","vnode":null}`,
	} {
		t.Run(input, func(t *testing.T) {
			var original Config
			require.NoError(t, json.Unmarshal([]byte(input), &original))
			for _, codec := range []struct {
				marshal   func(any) ([]byte, error)
				unmarshal func([]byte, any) error
			}{{json.Marshal, json.Unmarshal}, {yaml.Marshal, yaml.Unmarshal}} {
				data, err := codec.marshal(original)
				require.NoError(t, err)
				var got Config
				require.NoError(t, codec.unmarshal(data, &got))
				require.Equal(t, original, got)
			}
		})
	}
	for _, input := range []string{`{"vnode":1}`, `{"vnode":true}`, `{"vnode":[]}`} {
		var config Config
		require.Error(t, json.Unmarshal([]byte(input), &config))
		require.Error(t, yaml.Unmarshal([]byte(input), &config))
	}
}

func TestLegacyVnodeInputUsesCanonicalLocalConfiguration(t *testing.T) {
	for _, unmarshal := range []func([]byte, any) error{json.Unmarshal, yaml.Unmarshal} {
		c := New(ddsnmp.NewDeviceStore())
		require.NoError(t, unmarshal([]byte(`{"hostname":"device","vnode":{"hostname":"local","guid":"6e17cd2c-0518-4e94-965b-d25673decb21","labels":{"site":"test"}}}`), c))
		require.Empty(t, c.Vnode)
		require.True(t, c.CreateVnode, "legacy input preserves constructor defaults")
		require.Equal(t, defaultConfig().Options, c.Options)
		require.Equal(t, "local", c.LocalVnode.Hostname)
		require.Equal(t, "test", c.LocalVnode.Labels["site"])
		raw, err := json.Marshal(c.Configuration())
		require.NoError(t, err)
		var canonical map[string]any
		require.NoError(t, json.Unmarshal(raw, &canonical))
		require.Equal(t, "", canonical["vnode"])
		require.Equal(t, "local", canonical["local_vnode"].(map[string]any)["hostname"])
		var next Config
		require.NoError(t, json.Unmarshal(raw, &next))
		require.Equal(t, c.Config, next)
		for _, conflict := range []string{
			`{"vnode":{"hostname":"old"},"local_vnode":{"hostname":"new"}}`,
			`{"vnode":{},"local_vnode":{}}`,
		} {
			require.ErrorContains(t, unmarshal([]byte(conflict), &next), "cannot be combined")
		}
	}
}

func TestNamedVnodeDiagnosticsUseCanonicalIdentity(t *testing.T) {
	c := New(ddsnmp.NewDeviceStore())
	require.NoError(t, json.Unmarshal([]byte(`{"hostname":"192.0.2.1","vnode":"router"}`), c))
	c.SetConfiguredVnode(vnodes.VirtualNode{GUID: "6e17cd2c-0518-4e94-965b-d25673decb21", Hostname: "canonical", Labels: map[string]string{"site": "test"}})
	input := c.normalDeviceInput()
	require.Equal(t, "6e17cd2c-0518-4e94-965b-d25673decb21", input.VnodeGUID)
	require.Equal(t, "test", input.VnodeLabels["site"])
	input.VnodeLabels["site"] = "mutated"
	require.Equal(t, "test", c.normalDeviceInput().VnodeLabels["site"])
	require.Nil(t, c.VirtualNode(), "configured snapshots must not become collector-generated identities")
}
