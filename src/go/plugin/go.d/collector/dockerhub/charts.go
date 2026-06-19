// SPDX-License-Identifier: GPL-3.0-or-later

package dockerhub

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

type (
	// Charts is an alias for collectorapi.Charts
	Charts = collectorapi.Charts
	// Dims is an alias for collectorapi.Dims
	Dims = collectorapi.Dims
	// Dim is an alias for collectorapi.Dim
	Dim = collectorapi.Dim
)

var charts = Charts{
	{
		ID:    "pulls_sum",
		Title: "Pulls Summary",
		Units: "pulls",
		Fam:   "pulls",
		Dims: Dims{
			{ID: "pull_sum", Name: "sum"},
		},
	},
	{
		ID:    "pulls",
		Title: "Pulls",
		Units: "pulls",
		Fam:   "pulls",
		Type:  collectorapi.Stacked,
	},
	{
		ID:    "pulls_rate",
		Title: "Pulls Rate",
		Units: "pulls/s",
		Fam:   "pulls",
		Type:  collectorapi.Stacked,
	},
	{
		ID:    "stars",
		Title: "Stars",
		Units: "stars",
		Fam:   "stars",
		Type:  collectorapi.Stacked,
	},
	{
		ID:    "status",
		Title: "Current Status",
		Units: "status",
		Fam:   "status",
	},
	{
		ID:    "last_updated",
		Title: "Time Since Last Updated",
		Units: "seconds",
		Fam:   "last updated",
	},
}

func addReposToCharts(repositories []string, cs *Charts) {
	for _, name := range repositories {
		dimName := strings.ReplaceAll(name, "/", "_")
		_ = cs.Get("pulls").AddDim(&Dim{
			ID:   "pull_count_" + name,
			Name: dimName,
		})
		_ = cs.Get("pulls_rate").AddDim(&Dim{
			ID:   "pull_count_" + name,
			Name: dimName,
			Algo: collectorapi.Incremental,
		})
		_ = cs.Get("stars").AddDim(&Dim{
			ID:   "star_count_" + name,
			Name: dimName,
		})
		_ = cs.Get("status").AddDim(&Dim{
			ID:   "status_" + name,
			Name: dimName,
		})
		_ = cs.Get("last_updated").AddDim(&Dim{
			ID:   "last_updated_" + name,
			Name: dimName,
		})
	}
}
