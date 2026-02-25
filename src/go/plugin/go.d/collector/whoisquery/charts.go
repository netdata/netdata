// SPDX-License-Identifier: GPL-3.0-or-later

package whoisquery

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

var baseCharts = collectorapi.Charts{
	{
		ID:    "time_until_expiration",
		Title: "Time Until Domain Expiration",
		Units: "seconds",
		Fam:   "expiration time",
		Ctx:   "whoisquery.time_until_expiration",
		Opts:  collectorapi.Opts{StoreFirst: true},
		Dims: collectorapi.Dims{
			{ID: "expiry"},
		},
		Vars: collectorapi.Vars{
			{ID: "days_until_expiration_warning"},
			{ID: "days_until_expiration_critical"},
		},
	},
}
