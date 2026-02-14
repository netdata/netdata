// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import "github.com/cespare/xxhash/v2"

func seriesIDHash(id SeriesID) uint64 {
	if id == "" {
		return 0
	}
	return xxhash.Sum64String(string(id))
}
