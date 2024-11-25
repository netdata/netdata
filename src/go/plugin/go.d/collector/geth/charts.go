// SPDX-License-Identifier: GPL-3.0-or-later

package geth

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

type (
	Charts = module.Charts
	Chart  = module.Chart
	Dims   = module.Dims
	Dim    = module.Dim
)

var charts = Charts{
	chartAncientChainData.Copy(),
	chartChaindataDisk.Copy(),
	chartAncientChainDataRate.Copy(),
	chartChaindataDiskRate.Copy(),
	chartChainDataSize.Copy(),
	chartChainHead.Copy(),
	chartP2PNetwork.Copy(),
	chartNumberOfPeers.Copy(),
	chartp2pDialsServes.Copy(),
	chartReorgs.Copy(),
	chartReorgsBlocks.Copy(),
	chartGoRoutines.Copy(),
	chartTxPoolCurrent.Copy(),
	chartTxPoolQueued.Copy(),
	chartTxPoolPending.Copy(),
	chartRpcInformation.Copy(),
}

var (
	chartAncientChainDataRate = Chart{
		ID:    "chaindata_ancient_rate",
		Title: "Ancient Chaindata rate",
		Units: "bytes/s",
		Fam:   "chaindata",
		Ctx:   "geth.eth_db_chaindata_ancient_io_rate",
		Dims: Dims{
			{ID: ethDbChainDataAncientRead, Name: "reads", Algo: "incremental"},
			{ID: ethDbChainDataAncientWrite, Name: "writes", Mul: -1, Algo: "incremental"},
		},
	}

	chartAncientChainData = Chart{
		ID:    "chaindata_ancient",
		Title: "Session ancient Chaindata",
		Units: "bytes",
		Fam:   "chaindata",
		Ctx:   "geth.eth_db_chaindata_ancient_io",
		Dims: Dims{
			{ID: ethDbChainDataAncientRead, Name: "reads"},
			{ID: ethDbChainDataAncientWrite, Name: "writes", Mul: -1},
		},
	}
	chartChaindataDisk = Chart{
		ID:    "chaindata_disk",
		Title: "Session chaindata on disk",
		Units: "bytes",
		Fam:   "chaindata",
		Ctx:   "geth.eth_db_chaindata_disk_io",
		Dims: Dims{
			{ID: ethDbChaindataDiskRead, Name: "reads"},
			{ID: ethDbChainDataDiskWrite, Name: "writes", Mul: -1},
		},
	}
	chartGoRoutines = Chart{
		ID:    "goroutines",
		Title: "Number of goroutines",
		Units: "goroutines",
		Fam:   "goroutines",
		Ctx:   "geth.goroutines",
		Dims: Dims{
			{ID: goRoutines, Name: "goroutines"},
		},
	}
	chartChaindataDiskRate = Chart{
		ID:    "chaindata_disk_date",
		Title: "On disk Chaindata rate",
		Units: "bytes/s",
		Fam:   "chaindata",
		Ctx:   "geth.eth_db_chaindata_disk_io_rate",
		Dims: Dims{
			{ID: ethDbChaindataDiskRead, Name: "reads", Algo: "incremental"},
			{ID: ethDbChainDataDiskWrite, Name: "writes", Mul: -1, Algo: "incremental"},
		},
	}
	chartChainDataSize = Chart{
		ID:    "chaindata_db_size",
		Title: "Chaindata Size",
		Units: "bytes",
		Fam:   "chaindata",
		Ctx:   "geth.chaindata_db_size",
		Dims: Dims{
			{ID: ethDbChainDataDiskSize, Name: "levelDB"},
			{ID: ethDbChainDataAncientSize, Name: "ancientDB"},
		},
	}
	chartChainHead = Chart{
		ID:    "chainhead_overall",
		Title: "Chainhead",
		Units: "block",
		Fam:   "chainhead",
		Ctx:   "geth.chainhead",
		Dims: Dims{
			{ID: chainHeadBlock, Name: "block"},
			{ID: chainHeadReceipt, Name: "receipt"},
			{ID: chainHeadHeader, Name: "header"},
		},
	}
	chartTxPoolPending = Chart{
		ID:    "txpoolpending",
		Title: "Pending Transaction Pool",
		Units: "transactions",
		Fam:   "tx_pool",
		Ctx:   "geth.tx_pool_pending",
		Dims: Dims{
			{ID: txPoolInvalid, Name: "invalid"},
			{ID: txPoolPending, Name: "pending"},
			{ID: txPoolLocal, Name: "local"},
			{ID: txPoolPendingDiscard, Name: " discard"},
			{ID: txPoolNofunds, Name: "no funds"},
			{ID: txPoolPendingRatelimit, Name: "ratelimit"},
			{ID: txPoolPendingReplace, Name: "replace"},
		},
	}
	chartTxPoolCurrent = Chart{
		ID:    "txpoolcurrent",
		Title: "Transaction Pool",
		Units: "transactions",
		Fam:   "tx_pool",
		Ctx:   "geth.tx_pool_current",
		Dims: Dims{
			{ID: txPoolInvalid, Name: "invalid"},
			{ID: txPoolPending, Name: "pending"},
			{ID: txPoolLocal, Name: "local"},
			{ID: txPoolNofunds, Name: "pool"},
		},
	}
	chartTxPoolQueued = Chart{
		ID:    "txpoolqueued",
		Title: "Queued Transaction Pool",
		Units: "transactions",
		Fam:   "tx_pool",
		Ctx:   "geth.tx_pool_queued",
		Dims: Dims{
			{ID: txPoolQueuedDiscard, Name: "discard"},
			{ID: txPoolQueuedEviction, Name: "eviction"},
			{ID: txPoolQueuedNofunds, Name: "no_funds"},
			{ID: txPoolQueuedRatelimit, Name: "ratelimit"},
		},
	}
	chartP2PNetwork = Chart{
		ID:    "p2p_network",
		Title: "P2P bandwidth",
		Units: "bytes/s",
		Fam:   "p2p_bandwidth",
		Ctx:   "geth.p2p_bandwidth",
		Dims: Dims{
			{ID: p2pIngress, Name: "ingress", Algo: "incremental"},
			{ID: p2pEgress, Name: "egress", Mul: -1, Algo: "incremental"},
		},
	}
	chartReorgs = Chart{
		ID:    "reorgs_executed",
		Title: "Executed Reorgs",
		Units: "reorgs",
		Fam:   "reorgs",
		Ctx:   "geth.reorgs",
		Dims: Dims{
			{ID: reorgsExecuted, Name: "executed"},
		},
	}
	chartReorgsBlocks = Chart{
		ID:    "reorgs_blocks",
		Title: "Blocks Added/Removed from Reorg",
		Units: "blocks",
		Fam:   "reorgs",
		Ctx:   "geth.reorgs_blocks",
		Dims: Dims{
			{ID: reorgsAdd, Name: "added"},
			{ID: reorgsDropped, Name: "dropped"},
		},
	}

	chartNumberOfPeers = Chart{
		ID:    "p2p_peers_number",
		Title: "Number of Peers",
		Units: "peers",
		Fam:   "p2p_peers",
		Ctx:   "geth.p2p_peers",
		Dims: Dims{
			{ID: p2pPeers, Name: "peers"},
		},
	}

	chartp2pDialsServes = Chart{
		ID:    "p2p_dials_serves",
		Title: "P2P Serves and Dials",
		Units: "calls/s",
		Fam:   "p2p_peers",
		Ctx:   "geth.p2p_peers_calls",
		Dims: Dims{
			{ID: p2pDials, Name: "dials", Algo: "incremental"},
			{ID: p2pServes, Name: "serves", Algo: "incremental"},
		},
	}
	chartRpcInformation = Chart{
		ID:    "rpc_calls",
		Title: "rpc calls",
		Units: "calls/s",
		Fam:   "rpc",
		Ctx:   "geth.rpc_calls",
		Dims: Dims{
			{ID: rpcFailure, Name: "failed", Algo: "incremental"},
			{ID: rpcSuccess, Name: "successful", Algo: "incremental"},
		},
	}
)
