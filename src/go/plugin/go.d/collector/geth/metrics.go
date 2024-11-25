// SPDX-License-Identifier: GPL-3.0-or-later

package geth

// summary
const (
	chainValidation  = "chain_validation"
	chainWrite       = "chain_write"
	chainHeadBlock   = "chain_head_block"
	chainHeadHeader  = "chain_head_header"
	chainHeadReceipt = "chain_head_receipt"
)

// + rate
const (
	ethDbChainDataAncientRead  = "eth_db_chaindata_ancient_read"
	ethDbChainDataAncientWrite = "eth_db_chaindata_ancient_write"
	ethDbChaindataDiskRead     = "eth_db_chaindata_disk_read"
	ethDbChainDataDiskWrite    = "eth_db_chaindata_disk_write"
	ethDbChainDataDiskSize     = "eth_db_chaindata_disk_size"
	ethDbChainDataAncientSize  = "eth_db_chaindata_ancient_size"

	txPoolInvalid          = "txpool_invalid"
	txPoolPending          = "txpool_pending"
	txPoolLocal            = "txpool_local"
	txPoolPendingDiscard   = "txpool_pending_discard"
	txPoolNofunds          = "txpool_pending_nofunds"
	txPoolPendingRatelimit = "txpool_pending_ratelimit"
	txPoolPendingReplace   = "txpool_pending_replace"
	txPoolQueuedDiscard    = "txpool_queued_discard"
	txPoolQueuedEviction   = "txpool_queued_eviction"
	txPoolQueuedNofunds    = "txpool_queued_nofunds"
	txPoolQueuedRatelimit  = "txpool_queued_ratelimit"
)

const (
	// gauge
	p2pEgress  = "p2p_egress"
	p2pIngress = "p2p_ingress"

	p2pPeers  = "p2p_peers"
	p2pServes = "p2p_serves"
	p2pDials  = "p2p_dials"

	rpcRequests = "rpc_requests"
	rpcSuccess  = "rpc_success"
	rpcFailure  = "rpc_failure"

	reorgsAdd      = "chain_reorg_add"
	reorgsExecuted = "chain_reorg_executes"
	reorgsDropped  = "chain_reorg_drop"

	goRoutines = "system_cpu_goroutines"
)
