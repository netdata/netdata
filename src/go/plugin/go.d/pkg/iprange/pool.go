// SPDX-License-Identifier: GPL-3.0-or-later

package iprange

import (
	"math/big"
	"net"
	"strings"
)

// Pool is a collection of IP Ranges.
type Pool []Range

// String returns the string form of the pool.
func (p Pool) String() string {
	var b strings.Builder
	for _, r := range p {
		b.WriteString(r.String() + " ")
	}
	return strings.TrimSpace(b.String())
}

// Size reports the number of IP addresses in the pool.
func (p Pool) Size() *big.Int {
	size := big.NewInt(0)
	for _, r := range p {
		size.Add(size, r.Size())
	}
	return size
}

// Contains reports whether the pool includes IP.
func (p Pool) Contains(ip net.IP) bool {
	for _, r := range p {
		if r.Contains(ip) {
			return true
		}
	}
	return false
}
