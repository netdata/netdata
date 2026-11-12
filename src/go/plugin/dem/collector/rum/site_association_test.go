// SPDX-License-Identifier: GPL-3.0-or-later
package rum_test

import (
	"context"
	"encoding/json"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/rum"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
	"testing"
)

func TestNativeRUMVnodeReferenceConfiguration(t *testing.T) {
	for _, reference := range []string{"", "storefront", "site.name"} {
		t.Run(reference, func(t *testing.T) {
			factory, last := nativeRUMConfigFactory(t)
			input := confgroup.Config{"name": "physical-job", "allowed_origins": []any{"https://example.org"}, "vnode": reference}
			for _, format := range []string{"yaml", "json"} {
				var raw []byte
				var err error
				var cfg confgroup.Config
				if format == "yaml" {
					raw, err = yaml.Marshal(input)
					require.NoError(t, err)
					err = yaml.Unmarshal(raw, &cfg)
				} else {
					raw, err = json.Marshal(input)
					require.NoError(t, err)
					err = json.Unmarshal(raw, &cfg)
				}
				require.NoError(t, err)
				cfg.SetModule("rum").SetSourceType(confgroup.TypeUser)
				payload, err := factory.Configuration(context.Background(), cfg)
				require.NoError(t, err)
				actual := last().Configuration().(rum.Config)
				assert.Equal(t, reference, actual.Vnode)
				assert.Equal(t, "physical-job", actual.Name)
				assert.NotContains(t, string(payload), "vnode_guid", "resolved identity is not authored configuration")
			}
		})
	}
}
