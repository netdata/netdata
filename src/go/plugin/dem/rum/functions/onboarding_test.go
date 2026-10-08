// SPDX-License-Identifier: GPL-3.0-or-later
package functions_test

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	functions "github.com/netdata/netdata/go/plugins/plugin/dem/rum/functions"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	"github.com/stretchr/testify/require"
)

type onboardingSource struct {
	metadataFunctionSource
	site query.Site
}

func (s onboardingSource) Sites(context.Context) ([]query.Site, error) {
	return []query.Site{s.site}, nil
}
func TestSiteFunctionPublishesIndependentFacts(t *testing.T) {
	for _, received := range []bool{false, true} {
		site := query.Site{
			Name:              "shop",
			Generation:        "receiving-runtime",
			CollectionEnabled: true,
			Activity: query.Activity{
				LastBeaconAgeS: -1,
			},
		}
		if received {
			site.Activity.LastBeaconAt = time.Unix(1234, 0)
			site.Activity.LastBeaconAgeS = 7200
			site.ScriptURL = "https://receiver.example/a&b/rum/shop.js"
		}
		response := functions.New(onboardingSource{
			site: site,
		}).HandleRaw(context.Background(), funcapi.RawMethodRequest{
			Method: "rum-sites",
		}).RawResponse
		require.NotNil(t, response)
		columns := response["columns"].(map[string]any)
		rows := response["data"].([][]any)
		value := func(key string) any { return rows[0][columns[key].(map[string]any)["index"].(int)] }
		require.Equal(t, "receiving-runtime", value("generation"))
		require.Equal(t, 1, value("collection_enabled"))
		for _, key := range []string{"status", "reachable", "reach_detail", "snippet_check", "snippet_detail", "beacon_url"} {
			require.NotContains(t, columns, key)
		}
		require.Nil(t, value("last_rejected_at"))
		require.Nil(t, value("last_rejected_age_s"))
		for _, key := range []string{"last_beacon_at", "last_rejected_at"} {
			require.Equal(t, "datetime_usec", columns[key].(map[string]any)["value_options"].(map[string]any)["transform"])
		}
		if received {
			require.Equal(t, int64(1234000000), value("last_beacon_at"))
			require.Equal(t, 7200, value("last_beacon_age_s"))
			require.Equal(t, `<script async src="https://receiver.example/a&amp;b/rum/shop.js"></script>`, value("snippet"))
		} else {
			require.Nil(t, value("last_beacon_at"))
			require.Nil(t, value("last_beacon_age_s"))
			require.Nil(t, value("script_url"))
			require.Nil(t, value("snippet"))
		}
	}
}
