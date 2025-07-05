// SPDX-License-Identifier: GPL-3.0-or-later

package iprange

import (
	"fmt"
	"iter"
	"math/big"
	"net/netip"
)

// Family represents IP Range address-family.
type Family uint8

const (
	// V4Family is IPv4 address-family.
	V4Family Family = iota
	// V6Family is IPv6 address-family.
	V6Family
)

// String returns the string representation of the address family.
func (f Family) String() string {
	switch f {
	case V4Family:
		return "IPv4"
	case V6Family:
		return "IPv6"
	default:
		return fmt.Sprintf("Unknown(%d)", f)
	}
}

// Range represents an IP address range.
// It provides methods to check containment, iterate over addresses,
// and calculate the size of the range.
type Range interface {
	// Family returns the address family (IPv4 or IPv6) of the range.
	Family() Family

	// Contains reports whether the range includes the given IP address.
	Contains(ip netip.Addr) bool

	// Size returns the number of IP addresses in the range.
	Size() *big.Int

	// Iterate returns an iterator over all IP addresses in the range.
	Iterate() iter.Seq[netip.Addr]

	// String returns the string representation of the range in "start-end" format.
	fmt.Stringer

	// Start returns the first IP address in the range.
	Start() netip.Addr

	// End returns the last IP address in the range.
	End() netip.Addr
}

// New creates a new IP Range from start and end addresses.
// It returns nil if the range is invalid (start and end have different
// address families, or start > end).
func New(start, end netip.Addr) Range {
	if !start.IsValid() || !end.IsValid() {
		return nil
	}

	switch {
	case start.Is4() && end.Is4():
		if start.Compare(end) > 0 {
			return nil
		}
		return &v4Range{start: start, end: end}
	case start.Is6() && end.Is6() && !start.Is4In6() && !end.Is4In6():
		if start.Compare(end) > 0 {
			return nil
		}
		return &v6Range{start: start, end: end}
	default:
		return nil
	}
}

// v4Range implements Range for IPv4 addresses.
type v4Range struct {
	start netip.Addr
	end   netip.Addr
}

// compile-time check that v4Range implements Range
var _ Range = (*v4Range)(nil)

func (r *v4Range) Start() netip.Addr { return r.start }
func (r *v4Range) End() netip.Addr   { return r.end }

func (r *v4Range) Iterate() iter.Seq[netip.Addr] {
	return iterate(r)
}

func (r *v4Range) String() string {
	return fmt.Sprintf("%s-%s", r.start, r.end)
}

func (r *v4Range) Family() Family {
	return V4Family
}

func (r *v4Range) Contains(ip netip.Addr) bool {
	if !ip.Is4() {
		return false
	}
	return ip.Compare(r.start) >= 0 && ip.Compare(r.end) <= 0
}

func (r *v4Range) Size() *big.Int {
	// For IPv4, we can safely use uint32 arithmetic
	start := r.start.As4()
	end := r.end.As4()

	startUint := uint32(start[0])<<24 | uint32(start[1])<<16 | uint32(start[2])<<8 | uint32(start[3])
	endUint := uint32(end[0])<<24 | uint32(end[1])<<16 | uint32(end[2])<<8 | uint32(end[3])

	// Add 1 because the range is inclusive
	return big.NewInt(int64(endUint - startUint + 1))
}

// v6Range implements Range for IPv6 addresses.
type v6Range struct {
	start netip.Addr
	end   netip.Addr
}

// compile-time check that v6Range implements Range
var _ Range = (*v6Range)(nil)

func (r *v6Range) Start() netip.Addr { return r.start }
func (r *v6Range) End() netip.Addr   { return r.end }

func (r *v6Range) Iterate() iter.Seq[netip.Addr] {
	return iterate(r)
}

func (r *v6Range) String() string {
	return fmt.Sprintf("%s-%s", r.start, r.end)
}

func (r *v6Range) Family() Family {
	return V6Family
}

func (r *v6Range) Contains(ip netip.Addr) bool {
	if !ip.Is6() || ip.Is4In6() {
		return false
	}
	return ip.Compare(r.start) >= 0 && ip.Compare(r.end) <= 0
}

func (r *v6Range) Size() *big.Int {
	// For IPv6, we must use big.Int to handle 128-bit arithmetic
	startBytes := r.start.As16()
	endBytes := r.end.As16()

	startBig := new(big.Int).SetBytes(startBytes[:])
	endBig := new(big.Int).SetBytes(endBytes[:])

	// Calculate end - start + 1
	size := new(big.Int).Sub(endBig, startBig)
	size.Add(size, big.NewInt(1))

	return size
}
