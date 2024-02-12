// SPDX-License-Identifier: GPL-3.0-or-later

package energid

import "github.com/netdata/go.d.plugin/agent/module"

var charts = module.Charts{
	// getblockchaininfo (blockchain processing)
	{
		ID:    "blockindex",
		Title: "Blockchain index",
		Units: "count",
		Fam:   "blockchain",
		Ctx:   "energid.blockindex",
		Type:  module.Area,
		Dims: module.Dims{
			{ID: "blockchain_blocks", Name: "blocks"},
			{ID: "blockchain_headers", Name: "headers"},
		},
	},
	{
		ID:    "difficulty",
		Title: "Blockchain difficulty",
		Units: "difficulty",
		Fam:   "blockchain",
		Ctx:   "energid.difficulty",
		Dims: module.Dims{
			{ID: "blockchain_difficulty", Name: "difficulty", Div: 1000},
		},
	},

	// getmempoolinfo (state of the TX memory pool)
	{
		ID:    "mempool",
		Title: "Memory pool",
		Units: "bytes",
		Fam:   "memory",
		Ctx:   "energid.mempool",
		Type:  module.Area,
		Dims: module.Dims{
			{ID: "mempool_max", Name: "max"},
			{ID: "mempool_current", Name: "usage"},
			{ID: "mempool_txsize", Name: "tx_size"},
		},
	},

	// getmemoryinfo
	{
		ID:    "secmem",
		Title: "Secure memory",
		Units: "bytes",
		Fam:   "memory",
		Ctx:   "energid.secmem",
		Type:  module.Area,
		Dims: module.Dims{
			{ID: "secmem_total", Name: "total"},
			{ID: "secmem_used", Name: "used"},
			{ID: "secmem_free", Name: "free"},
			{ID: "secmem_locked", Name: "locked"},
		},
	},

	// getnetworkinfo (P2P networking)
	{
		ID:    "network",
		Title: "Network",
		Units: "connections",
		Fam:   "network",
		Ctx:   "energid.network",
		Dims: module.Dims{
			{ID: "network_connections", Name: "connections"},
		},
	},
	{
		ID:    "timeoffset",
		Title: "Network time offset",
		Units: "seconds",
		Fam:   "network",
		Ctx:   "energid.timeoffset",
		Dims: module.Dims{
			{ID: "network_timeoffset", Name: "timeoffset"},
		},
	},

	// gettxoutsetinfo (unspent transaction output set)
	{
		ID:    "utxo_transactions",
		Title: "Transactions",
		Units: "transactions",
		Fam:   "utxo",
		Ctx:   "energid.utxo_transactions",
		Dims: module.Dims{
			{ID: "utxo_transactions", Name: "transactions"},
			{ID: "utxo_output_transactions", Name: "output_transactions"},
		},
	},
}
