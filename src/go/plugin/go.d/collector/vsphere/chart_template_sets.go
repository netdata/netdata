// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

type legacyChartTemplateSet struct {
	charts collectorapi.Charts
}

func legacyChartTemplateSets() []legacyChartTemplateSet {
	return []legacyChartTemplateSet{
		{
			charts: inventoryChartsTmpl,
		},
		{
			charts: vmChartsTmpl,
		},
		{
			charts: hostChartsTmpl,
		},
		{
			charts: datastorePropertyChartsTmpl,
		},
		{
			charts: datastorePerfChartsTmpl,
		},
		{
			charts: clusterPropertyChartsTmpl,
		},
		{
			charts: clusterPerfChartsTmpl,
		},
		{
			charts: resourcePoolChartsTmpl,
		},
	}
}
