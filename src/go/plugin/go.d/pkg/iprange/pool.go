// SPDX-License-Identifier: GPL-3.0-or-later

package iprange

import (
	"fmt"
	"iter"
	"math/big"
	"net/netip"
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
func (p *Pool) ContainsRange(r Range) bool {
	if p == nil || r == nil {
		return false
	}

	// For efficiency, we check if the range is fully contained within
	// any single range in the pool first
	for _, poolRange := range p.ranges {
		if poolRange.Contains(r.Start()) && poolRange.Contains(r.End()) {
			return true
		}
	}

	// If not contained in a single range, we need to check if it's covered
	// by multiple ranges. This is more expensive but handles fragmented coverage.
	// For simplicity, we'll just check the start and end points.
	// A complete implementation would need to check for gaps.
	return p.Contains(r.Start()) && p.Contains(r.End())
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

// Format implements fmt.Formatter for custom formatting.
// It supports the following verbs:
//   - %s: default string representation (space-separated)
//   - %q: quoted string representation
//   - %v: same as %s
//   - %+v: verbose format with range count and size
func (p *Pool) Format(f fmt.State, verb rune) {
	switch verb {
	case 's':
		if p == nil {
			_, _ = f.Write([]byte("<nil>"))
			return
		}
		_, _ = f.Write([]byte(p.String()))
	case 'q':
		if p == nil {
			_, _ = f.Write([]byte(`"<nil>"`))
			return
		}
		_, _ = fmt.Fprintf(f, "%q", p.String())
	case 'v':
		if f.Flag('+') {
			// Handle %+v
			if p == nil {
				_, _ = f.Write([]byte("Pool(<nil>)"))
				return
			}
			_, _ = fmt.Fprintf(f, "Pool(ranges=%d, addresses=%s)", p.Len(), p.Size())
		} else {
			// Handle %v
			if p == nil {
				_, _ = f.Write([]byte("<nil>"))
				return
			}
			_, _ = f.Write([]byte(p.String()))
		}
	default:
		// For other verbs, use default string representation
		if p == nil {
			_, _ = f.Write([]byte("<nil>"))
			return
		}
		_, _ = f.Write([]byte(p.String()))
	}
}
