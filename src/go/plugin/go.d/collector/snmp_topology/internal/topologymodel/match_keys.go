// SPDX-License-Identifier: GPL-3.0-or-later

package topologymodel

import (
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func ChargeMatch(limiter worklimit.Limiter, match Match) error {
	if limiter == nil {
		return nil
	}
	for _, values := range [][]string{
		match.ChassisIDs,
		match.MacAddresses,
		match.IPAddresses,
		match.Hostnames,
		match.DNSNames,
		match.ContainerIDs,
		match.PodNames,
		match.NamespaceIDs,
		{match.SysName, match.SysObjectID},
	} {
		if err := worklimit.ChargeStrings(limiter, values); err != nil {
			return err
		}
	}
	return nil
}

func CanonicalMatchKey(match Match) string {
	key, _ := canonicalMatchKey(match, nil)
	return key
}

func canonicalMatchKey(match Match, limiter worklimit.Limiter) (string, error) {
	if key, err := canonicalPrimaryMACListKey(match, limiter); err != nil {
		return "", err
	} else if key != "" {
		return "mac:" + key, nil
	}
	if key, err := canonicalHardwareListKey(match.ChassisIDs, limiter); err != nil {
		return "", err
	} else if key != "" {
		return "chassis:" + key, nil
	}
	if key, err := canonicalIPListKey(match.IPAddresses, limiter); err != nil {
		return "", err
	} else if key != "" {
		return "ip:" + key, nil
	}
	if key, err := canonicalStringListKey(match.Hostnames, limiter); err != nil {
		return "", err
	} else if key != "" {
		return "hostname:" + key, nil
	}
	if key, err := canonicalStringListKey(match.DNSNames, limiter); err != nil {
		return "", err
	} else if key != "" {
		return "dns:" + key, nil
	}
	if sysName := strings.ToLower(strings.TrimSpace(match.SysName)); sysName != "" {
		return "sysname:" + sysName, nil
	}
	if match.SysObjectID != "" {
		return "sysobjectid:" + match.SysObjectID, nil
	}
	return "", nil
}

func LinkSortKey(link Link) string {
	key, _ := linkSortKey(link, nil)
	return key
}

func linkSortKey(link Link, limiter worklimit.Limiter) (string, error) {
	srcKey, err := canonicalMatchKey(link.Src.Match, limiter)
	if err != nil {
		return "", err
	}
	dstKey, err := canonicalMatchKey(link.Dst.Match, limiter)
	if err != nil {
		return "", err
	}
	parts := []string{
		link.Protocol,
		link.Direction,
		srcKey,
		dstKey,
		EndpointKey(link.Src, "if_index"),
		EndpointKey(link.Src, "if_name"),
		EndpointKey(link.Src, "port_id"),
		EndpointKey(link.Dst, "if_index"),
		EndpointKey(link.Dst, "if_name"),
		EndpointKey(link.Dst, "port_id"),
		link.State,
	}
	if err := worklimit.ChargeStrings(limiter, parts); err != nil {
		return "", err
	}
	return strings.Join(parts, "|"), nil
}

func SortActors(limiter worklimit.Limiter, actors []Actor) error {
	if limiter == nil {
		return worklimit.SortSlice(nil, actors, func(i, j int) bool {
			return CanonicalMatchKey(actors[i].Match) < CanonicalMatchKey(actors[j].Match)
		})
	}
	if err := limiter.Charge(uint64(len(actors))); err != nil {
		return err
	}
	type orderedActor struct {
		key   string
		actor Actor
	}
	ordered := make([]orderedActor, len(actors))
	for i, actor := range actors {
		key, err := canonicalMatchKey(actor.Match, limiter)
		if err != nil {
			return err
		}
		ordered[i] = orderedActor{key: key, actor: actor}
	}
	if err := worklimit.SortStableFunc(limiter, ordered, func(a, b orderedActor) int {
		return strings.Compare(a.key, b.key)
	}); err != nil {
		return err
	}
	if err := limiter.Charge(uint64(len(actors))); err != nil {
		return err
	}
	for i := range ordered {
		actors[i] = ordered[i].actor
	}
	return nil
}

func SortLinks(limiter worklimit.Limiter, links []Link) error {
	if limiter == nil {
		return worklimit.SortSlice(nil, links, func(i, j int) bool {
			return LinkSortKey(links[i]) < LinkSortKey(links[j])
		})
	}
	if err := limiter.Charge(uint64(len(links))); err != nil {
		return err
	}
	type orderedLink struct {
		key  string
		link Link
	}
	ordered := make([]orderedLink, len(links))
	for i, link := range links {
		key, err := linkSortKey(link, limiter)
		if err != nil {
			return err
		}
		ordered[i] = orderedLink{key: key, link: link}
	}
	if err := worklimit.SortStableFunc(limiter, ordered, func(a, b orderedLink) int {
		return strings.Compare(a.key, b.key)
	}); err != nil {
		return err
	}
	if err := limiter.Charge(uint64(len(links))); err != nil {
		return err
	}
	for i := range ordered {
		links[i] = ordered[i].link
	}
	return nil
}

func EndpointKey(endpoint LinkEndpoint, key string) string {
	switch key {
	case "if_index":
		if endpoint.IfIndex <= 0 {
			return ""
		}
		return strconv.Itoa(endpoint.IfIndex)
	case "if_name":
		return strings.TrimSpace(endpoint.IfName)
	case "port_id":
		return strings.TrimSpace(endpoint.PortID)
	case "port_name":
		return strings.TrimSpace(endpoint.PortName)
	default:
		return ""
	}
}

func MatchIdentityKeys(match Match) []string {
	seen := make(map[string]struct{}, 8)
	add := func(kind, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		seen[kind+":"+value] = struct{}{}
	}

	for _, value := range match.ChassisIDs {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if mac := topologyutil.NormalizeMAC(value); mac != "" {
			add("hw", mac)
			continue
		}
		if ip := topologyutil.NormalizeIPAddress(value); ip != "" {
			add("ip", ip)
			continue
		}
		add("chassis", strings.ToLower(value))
	}
	for _, value := range match.MacAddresses {
		if mac := topologyutil.NormalizeMAC(value); mac != "" {
			add("hw", mac)
		}
	}
	for _, value := range match.IPAddresses {
		if ip := topologyutil.NormalizeIPAddress(value); ip != "" {
			add("ip", ip)
			continue
		}
		add("ipraw", strings.ToLower(strings.TrimSpace(value)))
	}
	for _, value := range match.Hostnames {
		add("hostname", strings.ToLower(strings.TrimSpace(value)))
	}
	for _, value := range match.DNSNames {
		add("dns", strings.ToLower(strings.TrimSpace(value)))
	}
	if sysName := strings.TrimSpace(match.SysName); sysName != "" {
		add("sysname", strings.ToLower(sysName))
	}

	if len(seen) == 0 {
		return nil
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	_ = worklimit.SortStrings(nil, keys)
	return keys
}

func CanonicalPrimaryMACListKey(match Match) string {
	key, _ := canonicalPrimaryMACListKey(match, nil)
	return key
}

func canonicalPrimaryMACListKey(match Match, limiter worklimit.Limiter) (string, error) {
	if err := worklimit.ChargeStrings(limiter, match.MacAddresses); err != nil {
		return "", err
	}
	if err := worklimit.ChargeStrings(limiter, match.ChassisIDs); err != nil {
		return "", err
	}
	seen := make(map[string]struct{}, len(match.MacAddresses)+len(match.ChassisIDs))
	for _, value := range match.MacAddresses {
		if mac := topologyutil.NormalizeMAC(value); mac != "" {
			seen[mac] = struct{}{}
		}
	}
	for _, value := range match.ChassisIDs {
		if mac := topologyutil.NormalizeMAC(value); mac != "" {
			seen[mac] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return "", nil
	}
	values := make([]string, 0, len(seen))
	for value := range seen {
		values = append(values, value)
	}
	if err := worklimit.SortStrings(limiter, values); err != nil {
		return "", err
	}
	return strings.Join(values, ","), nil
}

func CanonicalHardwareListKey(values []string) string {
	key, _ := canonicalHardwareListKey(values, nil)
	return key
}

func canonicalHardwareListKey(values []string, limiter worklimit.Limiter) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	if err := worklimit.ChargeStrings(limiter, values); err != nil {
		return "", err
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if mac := topologyutil.NormalizeMAC(value); mac != "" {
			out = append(out, mac)
			continue
		}
		if ip := topologyutil.NormalizeIPAddress(value); ip != "" {
			out = append(out, ip)
			continue
		}
		out = append(out, strings.ToLower(value))
	}
	if len(out) == 0 {
		return "", nil
	}
	if err := worklimit.SortStrings(limiter, out); err != nil {
		return "", err
	}
	out = uniqueStrings(out)
	return strings.Join(out, ","), nil
}

func CanonicalMACListKey(values []string) string {
	key, _ := canonicalMACListKey(values, nil)
	return key
}

func canonicalMACListKey(values []string, limiter worklimit.Limiter) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	if err := worklimit.ChargeStrings(limiter, values); err != nil {
		return "", err
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if mac := topologyutil.NormalizeMAC(value); mac != "" {
			out = append(out, mac)
		}
	}
	if len(out) == 0 {
		return "", nil
	}
	if err := worklimit.SortStrings(limiter, out); err != nil {
		return "", err
	}
	out = uniqueStrings(out)
	return strings.Join(out, ","), nil
}

func CanonicalIPListKey(values []string) string {
	key, _ := canonicalIPListKey(values, nil)
	return key
}

func canonicalIPListKey(values []string, limiter worklimit.Limiter) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	if err := worklimit.ChargeStrings(limiter, values); err != nil {
		return "", err
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if ip := topologyutil.NormalizeIPAddress(value); ip != "" {
			out = append(out, ip)
			continue
		}
		out = append(out, strings.ToLower(value))
	}
	if len(out) == 0 {
		return "", nil
	}
	if err := worklimit.SortStrings(limiter, out); err != nil {
		return "", err
	}
	out = uniqueStrings(out)
	return strings.Join(out, ","), nil
}

func CanonicalStringListKey(values []string) string {
	key, _ := canonicalStringListKey(values, nil)
	return key
}

func canonicalStringListKey(values []string, limiter worklimit.Limiter) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	if err := worklimit.ChargeStrings(limiter, values); err != nil {
		return "", err
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return "", nil
	}
	if err := worklimit.SortStrings(limiter, out); err != nil {
		return "", err
	}
	out = uniqueStrings(out)
	return strings.Join(out, ","), nil
}

func uniqueStrings(values []string) []string {
	if len(values) <= 1 {
		return values
	}
	out := values[:0]
	var prev string
	for i, value := range values {
		if i == 0 || value != prev {
			out = append(out, value)
			prev = value
		}
	}
	return out
}
