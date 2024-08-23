// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

var (
	baseCharts = module.Charts{
		timeUntilExpirationChart.Copy(),
	}
	withRevocationCharts = module.Charts{
		timeUntilExpirationChart.Copy(),
		revocationStatusChart.Copy(),
	}

	timeUntilExpirationChart = module.Chart{
		ID:    "time_until_expiration",
		Title: "Time Until Certificate Expiration",
		Units: "seconds",
		Fam:   "expiration time",
		Ctx:   "x509check.time_until_expiration",
		Opts:  module.Opts{StoreFirst: true},
		Dims: module.Dims{
			{ID: "expiry"},
		},
		Vars: module.Vars{
			{ID: "days_until_expiration_warning"},
			{ID: "days_until_expiration_critical"},
		},
	}
	revocationStatusChart = module.Chart{
		ID:    "revocation_status",
		Title: "Revocation Status",
		Units: "boolean",
		Fam:   "revocation",
		Ctx:   "x509check.revocation_status",
		Opts:  module.Opts{StoreFirst: true},
		Dims: module.Dims{
			{ID: "not_revoked"},
			{ID: "revoked"},
		},
	}
)
