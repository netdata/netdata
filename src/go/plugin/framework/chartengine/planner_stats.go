// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

type planRouteStats struct {
	routeCacheHits       uint64
	routeCacheMisses     uint64
	seriesScanned        uint64
	seriesMatched        uint64
	seriesUnmatched      uint64
	seriesAutogenMatched uint64
	seriesFilteredBySeq  uint64
	seriesFilteredBySel  uint64
}
