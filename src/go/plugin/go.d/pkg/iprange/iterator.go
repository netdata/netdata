// SPDX-License-Identifier: GPL-3.0-or-later

package iprange

import (
	"iter"
	"net/netip"
)

// iterate returns an iterator that yields each IP address in the range.
// It handles both IPv4 and IPv6 ranges efficiently.
func iterate(r Range) iter.Seq[netip.Addr] {
	return func(yield func(netip.Addr) bool) {
		current := r.Start()
		end := r.End()

		// Handle empty or invalid range
		if !current.IsValid() || !end.IsValid() {
			return
		}

		for {
			// Yield current address
			if !yield(current) {
				return
			}

			// Check if we've reached the end
			if current == end {
				return
			}

			// Move to next address
			next := current.Next()

			// Check for overflow or going past the end
			if !next.IsValid() || next.Compare(end) > 0 {
				return
			}

			current = next
		}
	}
}
