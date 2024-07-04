// SPDX-License-Identifier: GPL-3.0-or-later

package ipfs

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioBW = module.Priority + iota
	prioPeers
	prioRepoSize
	prioRepoObj
)

var charts = module.Charts{
	bwChart.Copy(),
	peersChart.Copy(),
	repoSizeChart.Copy(),
	repoObjChart.Copy(),
}

var (
	bwChart = module.Chart{
		ID:       "bandwidth",
		Title:    "Bandwidth",
		Units:    "Kilobits/s",
		Fam:      "test",
		Ctx:      "ipfs.bandwidth",
		Type:     module.Area,
		Priority: prioBW,
		Dims: module.Dims{
			{ID: "TotalIn", Name: "in", Algo: module.Incremental},
			{ID: "TotalOut", Name: "out", Mul: -1, Algo: module.Incremental},
		},
	}

	peersChart = module.Chart{
		ID:       "peers",
		Title:    "Peers",
		Units:    "peers",
		Fam:      "test",
		Ctx:      "ipfs.peers",
		Type:     module.Line,
		Priority: prioPeers,
		Dims: module.Dims{
			{ID: "peers"},
		},
	}

	repoSizeChart = module.Chart{
		ID:       "repo_size",
		Title:    "IPFS Repo Size",
		Units:    "bytes",
		Fam:      "test",
		Ctx:      "ipfs.repo_size",
		Type:     module.Line,
		Priority: prioRepoSize,
		Dims: module.Dims{
			{ID: "avail"},
			{ID: "size"},
		},
	}

	repoObjChart = module.Chart{
		ID:       "repo_objects",
		Title:    "IPFS Repo Objects",
		Units:    "objects",
		Fam:      "test",
		Ctx:      "ipfs.repo_objects",
		Type:     module.Line,
		Priority: prioRepoObj,
		Dims: module.Dims{
			{ID: "pinned"},
			{ID: "recursive_pins"},
			{ID: "objects"},
		},
	}
)
