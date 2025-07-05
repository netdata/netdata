// SPDX-License-Identifier: GPL-3.0-or-later

package iprange

import (
	"iter"
	"math/big"
	"net/netip"
	"sort"
	"strings"
)

// Pool represents a collection of IP ranges.
// It provides methods to work with multiple ranges as a single unit.
type Pool struct {
	ranges []Range
}

// NewPool creates a new Pool from the given ranges.
// It filters out nil ranges and makes a defensive copy of the slice.
func NewPool(ranges ...Range) *Pool {
	filtered := make([]Range, 0, len(ranges))
	for _, r := range ranges {
		if r != nil {
			filtered = append(filtered, r)
		}
	}
	return &Pool{ranges: filtered}
}

// ParsePool parses a string containing multiple IP ranges and returns a Pool.
// The string should contain space-separated range specifications.
func ParsePool(s string) (*Pool, error) {
	ranges, err := ParseRanges(s)
	if err != nil {
		return nil, err
	}
	return NewPool(ranges...), nil
}

// Ranges returns a copy of the ranges in the pool.
func (p *Pool) Ranges() []Range {
	if p == nil || len(p.ranges) == 0 {
		return nil
	}

	result := make([]Range, len(p.ranges))
	copy(result, p.ranges)
	return result
}

// Len returns the number of ranges in the pool.
func (p *Pool) Len() int {
	if p == nil {
		return 0
	}
	return len(p.ranges)
}

// IsEmpty reports whether the pool contains no ranges.
func (p *Pool) IsEmpty() bool {
	return p.Len() == 0
}

// String returns the string representation of the pool.
// Ranges are separated by spaces.
func (p *Pool) String() string {
	if p == nil || len(p.ranges) == 0 {
		return ""
	}

	parts := make([]string, len(p.ranges))
	for i, r := range p.ranges {
		parts[i] = r.String()
	}
	return strings.Join(parts, " ")
}

// Size returns the total number of IP addresses across all ranges in the pool.
// Note: This does not account for overlapping ranges.
func (p *Pool) Size() *big.Int {
	if p == nil || len(p.ranges) == 0 {
		return big.NewInt(0)
	}

	total := new(big.Int)
	for _, r := range p.ranges {
		total.Add(total, r.Size())
	}
	return total
}

// Contains reports whether any range in the pool includes the given IP address.
func (p *Pool) Contains(ip netip.Addr) bool {
	if p == nil || !ip.IsValid() {
		return false
	}

	for _, r := range p.ranges {
		if r.Contains(ip) {
			return true
		}
	}
	return false
}

// ContainsRange reports whether the pool fully contains the given range.
// This is true if every IP in the given range is contained in at least one range in the pool.
// Note: For large ranges, this method may be expensive as it needs to verify coverage
// of all addresses in the range.
func (p *Pool) ContainsRange(r Range) bool {
	if p == nil || r == nil {
		return false
	}

	// Quick check: if start or end is not in pool, range can't be contained
	if !p.Contains(r.Start()) || !p.Contains(r.End()) {
		return false
	}

	// For efficiency, check if the range is fully contained within
	// any single range in the pool
	for _, poolRange := range p.ranges {
		if rangeFullyContains(poolRange, r) {
			return true
		}
	}

	// If not in a single range, we need to check if the range is covered
	// by multiple pool ranges without gaps.

	// For small ranges (up to 256 IPs), check every address for completeness.
	// This handles cases like a /24 network efficiently while being 100% accurate
	size := r.Size()
	if size.Cmp(big.NewInt(256)) <= 0 {
		for addr := range r.Iterate() {
			if !p.Contains(addr) {
				return false
			}
		}
		return true
	}

	// For larger ranges, we need a more efficient approach.
	// Sort pool ranges and check for coverage without gaps.
	return p.checkRangeCoverage(r)
}

// rangeFullyContains returns true if r1 fully contains r2
func rangeFullyContains(r1, r2 Range) bool {
	return r1.Start().Compare(r2.Start()) <= 0 && r1.End().Compare(r2.End()) >= 0
}

// checkRangeCoverage efficiently checks if a range is fully covered by the pool's ranges
func (p *Pool) checkRangeCoverage(r Range) bool {
	// Filter pool ranges that might overlap with r
	var relevant []Range
	for _, pr := range p.ranges {
		// Check if pr overlaps with r
		if pr.End().Compare(r.Start()) >= 0 && pr.Start().Compare(r.End()) <= 0 {
			relevant = append(relevant, pr)
		}
	}

	if len(relevant) == 0 {
		return false
	}

	sort.Slice(relevant, func(i, j int) bool {
		return relevant[i].Start().Compare(relevant[j].Start()) < 0
	})

	// Check if the sorted ranges cover r without gaps
	currentCoverage := r.Start()

	for _, pr := range relevant {
		// If there's a gap between current coverage and this range
		if pr.Start().Compare(currentCoverage) > 0 {
			return false
		}

		// Extend coverage if this range extends it
		if pr.End().Compare(currentCoverage) >= 0 {
			currentCoverage = pr.End()

			// If we've covered the entire range, we're done
			if currentCoverage.Compare(r.End()) >= 0 {
				return true
			}

			// Move to next address for continuous coverage check
			next := currentCoverage.Next()
			if next.IsValid() {
				currentCoverage = next
			}
		}
	}

	// Check if we covered up to or past the end
	return currentCoverage.Compare(r.End()) > 0
}

// Iterate returns an iterator over all IP addresses in all ranges.
// Note: If ranges overlap, addresses in the overlap will be yielded multiple times.
func (p *Pool) Iterate() iter.Seq[netip.Addr] {
	return func(yield func(netip.Addr) bool) {
		if p == nil {
			return
		}

		for _, r := range p.ranges {
			for addr := range r.Iterate() {
				if !yield(addr) {
					return
				}
			}
		}
	}
}

// IterateRanges returns an iterator over all ranges in the pool.
func (p *Pool) IterateRanges() iter.Seq[Range] {
	return func(yield func(Range) bool) {
		if p == nil {
			return
		}

		for _, r := range p.ranges {
			if !yield(r) {
				return
			}
		}
	}
}

// Add appends one or more ranges to the pool.
func (p *Pool) Add(ranges ...Range) {
	for _, r := range ranges {
		if r != nil {
			p.ranges = append(p.ranges, r)
		}
	}
}

// AddString parses the string as IP ranges and adds them to the pool.
func (p *Pool) AddString(s string) error {
	ranges, err := ParseRanges(s)
	if err != nil {
		return err
	}
	p.Add(ranges...)
	return nil
}

// Clone returns a deep copy of the pool.
func (p *Pool) Clone() *Pool {
	if p == nil {
		return nil
	}
	return NewPool(p.ranges...)
}
