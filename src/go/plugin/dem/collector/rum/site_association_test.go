// SPDX-License-Identifier: GPL-3.0-or-later

package rum_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/rum"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestNativeRUMSiteAssociationConfig(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  string
		valid bool
	}{
		{"", "", true}, {"null", "", true}, {`""`, "", true}, {"storefront-1_web", "storefront-1_web", true},
		{strings.Repeat("a", 64), strings.Repeat("a", 64), true},
		{"Upper", "", false}, {"-first", "", false}, {"has.dot", "", false}, {`"has space"`, "", false}, {strings.Repeat("a", 65), "", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			factory, last := nativeRUMConfigFactory(t)
			raw := "name: physical-job\nallowed_origins: [https://example.org]\n"
			if tc.value != "" {
				raw += "site_id: " + tc.value + "\n"
			}
			for _, format := range []string{"yaml", "json"} {
				var cfg confgroup.Config
				require.NoError(t, yaml.Unmarshal([]byte(raw), &cfg))
				if format == "json" {
					b, err := json.Marshal(cfg)
					require.NoError(t, err)
					cfg = nil
					require.NoError(t, json.Unmarshal(b, &cfg))
				}
				cfg.SetModule("rum").SetSourceType(confgroup.TypeUser)
				err := factory.Test(context.Background(), cfg)
				if !tc.valid {
					require.ErrorContains(t, err, "site_id")
					continue
				}
				require.NoError(t, err)
				actual := last().Configuration().(rum.Config)
				assert.Equal(t, tc.want, actual.SiteID)
				assert.Equal(t, "physical-job", actual.Name, "association never replaces beacon/job identity")
			}
		})
	}
}
