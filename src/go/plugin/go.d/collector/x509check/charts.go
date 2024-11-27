// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"fmt"
	"strconv"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var certChartsTmpl = module.Charts{
	certTimeUntilExpirationChartTmpl.Copy(),
	certRevocationStatusChartTmpl.Copy(),
}

var (
	certTimeUntilExpirationChartTmpl = module.Chart{
		ID:    "cert_depth%d_time_until_expiration",
		Title: "Time Until Certificate Expiration",
		Units: "seconds",
		Fam:   "expiration time",
		Ctx:   "x509check.time_until_expiration",
		Opts:  module.Opts{StoreFirst: true},
		Dims: module.Dims{
			{ID: "cert_depth%d_expiry", Name: "expiry"},
		},
	}
	certRevocationStatusChartTmpl = module.Chart{
		ID:    "cert_depth%d_revocation_status",
		Title: "Revocation Status",
		Units: "boolean",
		Fam:   "revocation",
		Ctx:   "x509check.revocation_status",
		Opts:  module.Opts{StoreFirst: true},
		Dims: module.Dims{
			{ID: "cert_depth%d_not_revoked", Name: "not_revoked"},
			{ID: "cert_depth%d_revoked", Name: "revoked"},
		},
	}
)

func (c *Collector) addCertCharts(commonName string, depth int) {
	charts := certChartsTmpl.Copy()

	if depth > 0 || !c.CheckRevocation {
		_ = charts.Remove(certRevocationStatusChartTmpl.ID)
	}

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, depth)
		chart.Labels = []module.Label{
			{Key: "source", Value: c.Source},
			{Key: "common_name", Value: commonName},
			{Key: "depth", Value: strconv.Itoa(depth)},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, depth)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warningf("failed to add charts for '%s': %v", commonName, err)
	}
}
