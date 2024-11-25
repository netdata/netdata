// SPDX-License-Identifier: GPL-3.0-or-later

package ipfs

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioBandwidth = module.Priority + iota
	prioSwarmPeers
	prioDatastoreSpaceUtilization
	prioRepoSize
	prioRepoObj
	prioRepoPinnedObj
)

var charts = module.Charts{
	bandwidthChart.Copy(),
	peersChart.Copy(),
	datastoreUtilizationChart.Copy(),
	repoSizeChart.Copy(),
	repoObjChart.Copy(),
	repoPinnedObjChart.Copy(),
}

var (
	bandwidthChart = module.Chart{
		ID:       "bandwidth",
		Title:    "IPFS Bandwidth",
		Units:    "bytes/s",
		Fam:      "bandwidth",
		Ctx:      "ipfs.bandwidth",
		Type:     module.Area,
		Priority: prioBandwidth,
		Dims: module.Dims{
			{ID: "in", Algo: module.Incremental},
			{ID: "out", Mul: -1, Algo: module.Incremental},
		},
	}

	peersChart = module.Chart{
		ID:       "peers",
		Title:    "IPFS Peers",
		Units:    "peers",
		Fam:      "peers",
		Ctx:      "ipfs.peers",
		Type:     module.Line,
		Priority: prioSwarmPeers,
		Dims: module.Dims{
			{ID: "peers"},
		},
	}

	datastoreUtilizationChart = module.Chart{
		ID:       "datastore_space_utilization",
		Title:    "IPFS Datastore Space Utilization",
		Units:    "percent",
		Fam:      "size",
		Ctx:      "ipfs.datastore_space_utilization",
		Type:     module.Area,
		Priority: prioDatastoreSpaceUtilization,
		Dims: module.Dims{
			{ID: "used_percent", Name: "used"},
		},
	}
	repoSizeChart = module.Chart{
		ID:       "repo_size",
		Title:    "IPFS Repo Size",
		Units:    "bytes",
		Fam:      "size",
		Ctx:      "ipfs.repo_size",
		Type:     module.Line,
		Priority: prioRepoSize,
		Dims: module.Dims{
			{ID: "size"},
		},
	}

	repoObjChart = module.Chart{
		ID:       "repo_objects",
		Title:    "IPFS Repo Objects",
		Units:    "objects",
		Fam:      "objects",
		Ctx:      "ipfs.repo_objects",
		Type:     module.Line,
		Priority: prioRepoObj,
		Dims: module.Dims{
			{ID: "objects"},
		},
	}
	repoPinnedObjChart = module.Chart{
		ID:       "repo_pinned_objects",
		Title:    "IPFS Repo Pinned Objects",
		Units:    "objects",
		Fam:      "objects",
		Ctx:      "ipfs.repo_pinned_objects",
		Type:     module.Line,
		Priority: prioRepoPinnedObj,
		Dims: module.Dims{
			{ID: "pinned"},
			{ID: "recursive_pins"},
		},
	}
)
