// SPDX-License-Identifier: GPL-3.0-or-later

package vnodes

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestAuthoredConfigRoundtripAndResolution(t *testing.T) {
	var c Config
	require.NoError(t, yaml.Unmarshal([]byte(`name: router
mode: snmp
labels:
  site: configured
mode_snmp:
  address: router.example
  credentials:
    community: test-secret
  retries: 0
`), &c))
	c.SourceType = "stock"
	require.NoError(t, c.Validate())
	require.Nil(t, c.Resolve(nil))
	defaults := c.ModeSNMP.Defaults()
	require.Equal(t, 161, defaults.Port)
	require.Equal(t, 5*time.Second, defaults.Timeout.Duration())
	require.Equal(t, 0, *defaults.Retries)
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			var raw []byte
			var err error
			var decoded Config
			if format == "json" {
				raw, err = json.Marshal(c)
				require.NoError(t, err)
				err = json.Unmarshal(raw, &decoded)
			} else {
				raw, err = yaml.Marshal(c)
				require.NoError(t, err)
				err = yaml.Unmarshal(raw, &decoded)
			}
			require.NoError(t, err)
			decoded.SourceType = c.SourceType
			require.Equal(t, c, decoded)
		})
	}
	meta := &Metadata{Hostname: "device-name", Labels: map[string]string{"site": "acquired", "model": "router"}}
	resolved := c.Resolve(meta)
	require.Equal(t, "device-name", resolved.Hostname)
	require.Equal(t, "configured", resolved.Labels["site"])
	require.Equal(t, uuid.NewSHA1(uuid.NameSpaceDNS, []byte("router.example")).String(), resolved.GUID)
	raw, err := json.Marshal(resolved)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "secret")
	require.NotContains(t, string(raw), "mode_snmp")
	copy := c.Copy()
	delete(copy.Labels, "site")
	require.Equal(t, "acquired", copy.Resolve(meta).Labels["site"])
	copy.Hostname = "override"
	require.Equal(t, "override", copy.Resolve(meta).Hostname)
	require.Equal(t, "router", c.Resolve(&Metadata{}).Hostname)
	require.Equal(t, "acquired", meta.Labels["site"])
}
func TestModeAwareUpdate(t *testing.T) {
	var c Config
	require.NoError(t, json.Unmarshal([]byte(`{"name":"router","mode":"snmp","mode_snmp":{"address":"router","credentials":{"community":"old"}}}`), &c))
	c.SourceType = "stock"
	require.NoError(t, c.Validate())
	next := c.Copy()
	next.Labels = map[string]string{"site": "new"}
	require.NoError(t, c.ValidateUpdate(next))
	require.True(t, c.SameAcquisition(next))
	next.ModeSNMP.Credentials.Community = "new"
	require.NoError(t, c.ValidateUpdate(next))
	require.False(t, c.SameAcquisition(next))
	next.ModeSNMP.Address = "replacement"
	require.Error(t, c.ValidateUpdate(next))
	next = c.Copy()
	next.GUID = uuid.NewString()
	require.Error(t, c.ValidateUpdate(next))
	next = &Config{VirtualNode: VirtualNode{Name: "router", Hostname: "router", GUID: uuid.NewString()}}
	require.Error(t, c.ValidateUpdate(next))
	require.Error(t, next.ValidateUpdate(&c))
}
func TestSNMPNameRequired(t *testing.T) {
	var c Config
	require.NoError(t, yaml.Unmarshal([]byte("mode: snmp\nmode_snmp:\n  address: router\n  credentials:\n    community: secret\n"), &c))
	require.ErrorContains(t, c.Validate(), "name is required")
}

func TestResolveUsesEffectiveHostnameForFallbackAndLabel(t *testing.T) {
	var c Config
	require.NoError(t, json.Unmarshal([]byte(`{"name":"stable-name","mode":"snmp","mode_snmp":{"address":"device","credentials":{"community":"fixture"}}}`), &c))
	c.SourceType = "user"
	require.NoError(t, c.Validate())
	t.Run("fallback", func(t *testing.T) {
		fallback := c.Resolve(&Metadata{Hostname: "   ", Labels: map[string]string{"description": "usable"}})
		require.Equal(t, "stable-name", fallback.Hostname)
		require.NoError(t, ValidateConfigured(fallback))
	})
	t.Run("override", func(t *testing.T) {
		c.Hostname = "override"
		effective := c.Resolve(&Metadata{Hostname: "device-name"})
		require.Equal(t, "override", effective.Hostname)
		require.Equal(t, "override", effective.Labels["_hostname"])
	})
}
