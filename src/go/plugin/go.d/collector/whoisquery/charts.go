// SPDX-License-Identifier: GPL-3.0-or-later

package whoisquery

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

var baseCharts = module.Charts{
	{
		ID:    "time_until_expiration",
		Title: "Time Until Domain Expiration",
		Units: "seconds",
		Fam:   "expiration time",
		Ctx:   "whoisquery.time_until_expiration",
		Opts:  module.Opts{StoreFirst: true},
		Dims: module.Dims{
			{ID: "expiry"},
		},
		Vars: module.Vars{
			{ID: "days_until_expiration_warning"},
			{ID: "days_until_expiration_critical"},
		},
	},
}
