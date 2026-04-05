// SPDX-License-Identifier: GPL-3.0-or-later

package ipfs

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioBandwidth = collectorapi.Priority + iota
	prioSwarmPeers
	prioDatastoreSpaceUtilization
	prioRepoSize
	prioRepoObj
	prioRepoPinnedObj
)

var charts = collectorapi.Charts{
	bandwidthChart.Copy(),
	peersChart.Copy(),
	datastoreUtilizationChart.Copy(),
	repoSizeChart.Copy(),
	repoObjChart.Copy(),
	repoPinnedObjChart.Copy(),
}

var (
	bandwidthChart = collectorapi.Chart{
		ID:       "bandwidth",
		Title:    "IPFS Bandwidth",
		Units:    "bytes/s",
		Fam:      "bandwidth",
		Ctx:      "ipfs.bandwidth",
		Type:     collectorapi.Area,
		Priority: prioBandwidth,
		Dims: collectorapi.Dims{
			{ID: "in", Algo: collectorapi.Incremental},
			{ID: "out", Mul: -1, Algo: collectorapi.Incremental},
		},
	}

	peersChart = collectorapi.Chart{
		ID:       "peers",
		Title:    "IPFS Peers",
		Units:    "peers",
		Fam:      "peers",
		Ctx:      "ipfs.peers",
		Type:     collectorapi.Line,
		Priority: prioSwarmPeers,
		Dims: collectorapi.Dims{
			{ID: "peers"},
		},
	}

	datastoreUtilizationChart = collectorapi.Chart{
		ID:       "datastore_space_utilization",
		Title:    "IPFS Datastore Space Utilization",
		Units:    "percent",
		Fam:      "size",
		Ctx:      "ipfs.datastore_space_utilization",
		Type:     collectorapi.Area,
		Priority: prioDatastoreSpaceUtilization,
		Dims: collectorapi.Dims{
			{ID: "used_percent", Name: "used"},
		},
	}
	repoSizeChart = collectorapi.Chart{
		ID:       "repo_size",
		Title:    "IPFS Repo Size",
		Units:    "bytes",
		Fam:      "size",
		Ctx:      "ipfs.repo_size",
		Type:     collectorapi.Line,
		Priority: prioRepoSize,
		Dims: collectorapi.Dims{
			{ID: "size"},
		},
	}

	repoObjChart = collectorapi.Chart{
		ID:       "repo_objects",
		Title:    "IPFS Repo Objects",
		Units:    "objects",
		Fam:      "objects",
		Ctx:      "ipfs.repo_objects",
		Type:     collectorapi.Line,
		Priority: prioRepoObj,
		Dims: collectorapi.Dims{
			{ID: "objects"},
		},
	}
	repoPinnedObjChart = collectorapi.Chart{
		ID:       "repo_pinned_objects",
		Title:    "IPFS Repo Pinned Objects",
		Units:    "objects",
		Fam:      "objects",
		Ctx:      "ipfs.repo_pinned_objects",
		Type:     collectorapi.Line,
		Priority: prioRepoPinnedObj,
		Dims: collectorapi.Dims{
			{ID: "pinned"},
			{ID: "recursive_pins"},
		},
	}
)
