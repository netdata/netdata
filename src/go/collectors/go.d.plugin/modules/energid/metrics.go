// SPDX-License-Identifier: GPL-3.0-or-later

package energid

// API docs: https://github.com/energicryptocurrency/core-api-documentation

type energidInfo struct {
	Blockchain *blockchainInfo `stm:"blockchain"`
	MemPool    *memPoolInfo    `stm:"mempool"`
	Network    *networkInfo    `stm:"network"`
	TxOutSet   *txOutSetInfo   `stm:"utxo"`
	Memory     *memoryInfo     `stm:"secmem"`
}

// https://github.com/energicryptocurrency/core-api-documentation#getblockchaininfo
type blockchainInfo struct {
	Blocks     float64 `stm:"blocks" json:"blocks"`
	Headers    float64 `stm:"headers" json:"headers"`
	Difficulty float64 `stm:"difficulty,1000,1" json:"difficulty"`
}

// https://github.com/energicryptocurrency/core-api-documentation#getmempoolinfo
type memPoolInfo struct {
	Bytes      float64 `stm:"txsize" json:"bytes"`
	Usage      float64 `stm:"current" json:"usage"`
	MaxMemPool float64 `stm:"max" json:"maxmempool"`
}

// https://github.com/energicryptocurrency/core-api-documentation#getnetworkinfo
type networkInfo struct {
	TimeOffset  float64 `stm:"timeoffset" json:"timeoffset"`
	Connections float64 `stm:"connections" json:"connections"`
}

// https://github.com/energicryptocurrency/core-api-documentation#gettxoutsetinfo
type txOutSetInfo struct {
	Transactions float64 `stm:"transactions" json:"transactions"`
	TxOuts       float64 `stm:"output_transactions" json:"txouts"`
}

// undocumented
type memoryInfo struct {
	Locked struct {
		Used   float64 `stm:"used" json:"used"`
		Free   float64 `stm:"free" json:"free"`
		Total  float64 `stm:"total" json:"total"`
		Locked float64 `stm:"locked" json:"locked"`
	} `stm:"" json:"locked"`
}
