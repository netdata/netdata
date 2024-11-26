// SPDX-License-Identifier: GPL-3.0-or-later

package dockerhub

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

type (
	// Charts is an alias for module.Charts
	Charts = module.Charts
	// Dims is an alias for module.Dims
	Dims = module.Dims
	// Dim is an alias for module.Dim
	Dim = module.Dim
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
		Type:  module.Stacked,
	},
	{
		ID:    "pulls_rate",
		Title: "Pulls Rate",
		Units: "pulls/s",
		Fam:   "pulls",
		Type:  module.Stacked,
	},
	{
		ID:    "stars",
		Title: "Stars",
		Units: "stars",
		Fam:   "stars",
		Type:  module.Stacked,
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
		dimName := strings.Replace(name, "/", "_", -1)
		_ = cs.Get("pulls").AddDim(&Dim{
			ID:   "pull_count_" + name,
			Name: dimName,
		})
		_ = cs.Get("pulls_rate").AddDim(&Dim{
			ID:   "pull_count_" + name,
			Name: dimName,
			Algo: module.Incremental,
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
