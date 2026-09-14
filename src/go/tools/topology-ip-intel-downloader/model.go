// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
)

type ipClass string

const (
	ipClassPublic      ipClass = "public"
	ipClassInteresting ipClass = "interesting"
	ipClassPrivate     ipClass = "private"
	ipClassLocalhost   ipClass = "localhost"
)

type asnRange struct {
	start netip.Addr
	end   netip.Addr
	asn   uint32
	org   string
}

type geoRange struct {
	start netip.Addr
	end   netip.Addr

	country string
	state   string
	city    string

	latitude    float64
	longitude   float64
	hasLocation bool
}

type classification struct {
	prefixes []netip.Prefix
	class    ipClass
}

func (r asnRange) validate() error {
	if !r.start.IsValid() || !r.end.IsValid() {
		return errors.New("invalid range address")
	}
	if r.start.BitLen() != r.end.BitLen() {
		return fmt.Errorf("mixed address family range: %s-%s", r.start, r.end)
	}
	if compareAddrs(r.start, r.end) > 0 {
		return fmt.Errorf("range start %s is after end %s", r.start, r.end)
	}
	return nil
}

func (r geoRange) validate() error {
	if !r.start.IsValid() || !r.end.IsValid() {
		return errors.New("invalid range address")
	}
	if r.start.BitLen() != r.end.BitLen() {
		return fmt.Errorf("mixed address family range: %s-%s", r.start, r.end)
	}
	if compareAddrs(r.start, r.end) > 0 {
		return fmt.Errorf("range start %s is after end %s", r.start, r.end)
	}
	if r.country != "" && len(r.country) != 2 {
		return fmt.Errorf("country code must be 2 chars: %q", r.country)
	}
	return nil
}

func compareAddrs(a, b netip.Addr) int {
	aa := a.As16()
	bb := b.As16()
	return bytes.Compare(aa[:], bb[:])
}

func normalizeCountry(country string) string {
	country = strings.TrimSpace(strings.ToUpper(country))
	if len(country) != 2 {
		return ""
	}
	if country == "--" {
		return ""
	}
	for _, r := range country {
		if r < 'A' || r > 'Z' {
			return ""
		}
	}
	return country
}

func classifyRanges(policy policyConfig) ([]classification, error) {
	res := make([]classification, 0, 3)
	appendSet := func(cidrs []string, class ipClass) error {
		if len(cidrs) == 0 {
			return nil
		}
		prefixes := make([]netip.Prefix, 0, len(cidrs))
		for _, cidr := range cidrs {
			cidr = strings.TrimSpace(cidr)
			if cidr == "" {
				continue
			}
			p, err := netip.ParsePrefix(cidr)
			if err != nil {
				return fmt.Errorf("invalid %s cidr %q: %w", class, cidr, err)
			}
			prefixes = append(prefixes, p.Masked())
		}
		if len(prefixes) == 0 {
			return nil
		}

		sort.Slice(prefixes, func(i, j int) bool {
			if prefixes[i].Addr().BitLen() != prefixes[j].Addr().BitLen() {
				return prefixes[i].Addr().BitLen() < prefixes[j].Addr().BitLen()
			}
			if prefixes[i].Bits() != prefixes[j].Bits() {
				return prefixes[i].Bits() < prefixes[j].Bits()
			}
			return compareAddrs(prefixes[i].Addr(), prefixes[j].Addr()) < 0
		})
		res = append(res, classification{prefixes: prefixes, class: class})
		return nil
	}

	if err := appendSet(policy.interestingCIDRs, ipClassInteresting); err != nil {
		return nil, err
	}
	if err := appendSet(policy.privateCIDRs, ipClassPrivate); err != nil {
		return nil, err
	}
	if err := appendSet(policy.localhostCIDRs, ipClassLocalhost); err != nil {
		return nil, err
	}
	return res, nil
}

func trackIndividual(class ipClass) bool {
	return class == ipClassInteresting || class == ipClassPrivate || class == ipClassLocalhost
}
