// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

const (
	selectionEntitySite      = "site"
	selectionEntityInterface = "interface"
	selectionEntityBGPPeer   = "bgp_peer"

	selectionSkipSelector = "selector"
	selectionSkipLimit    = "limit"
)

type entitySelector struct {
	include []matcher.Matcher
	exclude []matcher.Matcher
}

func (c *Collector) initEntitySelectors() error {
	var err error
	if c.siteMatcher, err = newEntitySelector("site_selector", c.SiteSelector); err != nil {
		return err
	}
	if c.interfaceMatcher, err = newEntitySelector("interface_selector", c.InterfaceSelector); err != nil {
		return err
	}
	if c.bgpPeerMatcher, err = newEntitySelector("bgp.peer_selector", c.BGP.PeerSelector); err != nil {
		return err
	}
	return nil
}

func newEntitySelector(name, expr string) (*entitySelector, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" || expr == "*" {
		return nil, nil
	}
	selector := &entitySelector{}
	for _, term := range strings.Fields(expr) {
		positive := true
		if strings.HasPrefix(term, "!") {
			positive = false
			term = strings.TrimPrefix(term, "!")
		}
		if term == "" {
			return nil, fmt.Errorf("init %s: empty selector term", name)
		}
		m, err := matcher.NewGlobMatcher(term)
		if err != nil {
			return nil, fmt.Errorf("init %s: %w", name, err)
		}
		if positive {
			selector.include = append(selector.include, m)
		} else {
			selector.exclude = append(selector.exclude, m)
		}
	}
	if len(selector.include) == 0 {
		m, err := matcher.NewGlobMatcher("*")
		if err != nil {
			return nil, fmt.Errorf("init %s: %w", name, err)
		}
		selector.include = append(selector.include, m)
	}
	return selector, nil
}

func (s *entitySelector) matches(values ...string) bool {
	if s == nil {
		return true
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		for _, m := range s.exclude {
			if m.MatchString(value) {
				return false
			}
		}
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		for _, m := range s.include {
			if m.MatchString(value) {
				return true
			}
		}
	}
	return false
}

func (c *Collector) selectSites(siteIDs []string, siteNames map[string]string) ([]string, int, int) {
	limit := c.maxSitesLimit()
	selected := make([]string, 0, len(siteIDs))
	var skippedSelector, skippedLimit int

	for _, siteID := range siteIDs {
		if !c.siteMatcher.matches(siteID, siteNames[siteID]) {
			skippedSelector++
			continue
		}
		if limit > 0 && len(selected) >= limit {
			skippedLimit++
			continue
		}
		selected = append(selected, siteID)
	}

	return selected, skippedSelector, skippedLimit
}

func (c *Collector) applyEntityControls(sites map[string]*siteState, order *[]string) {
	c.ensureHealth()

	c.pruneUnselectedSites(sites, order)
	c.updateSiteSelectionHealth()

	var (
		selectedInterfaces, skippedInterfacesBySelector, skippedInterfacesByLimit int
		selectedBGPPeers, skippedBGPPeersBySelector, skippedBGPPeersByLimit       int
	)

	for _, siteID := range *order {
		site := sites[siteID]
		if site == nil {
			continue
		}

		selected, skippedSelector, skippedLimit := c.filterSiteInterfaces(site)
		selectedInterfaces += selected
		skippedInterfacesBySelector += skippedSelector
		skippedInterfacesByLimit += skippedLimit

		selected, skippedSelector, skippedLimit = c.filterSiteBGPPeers(site)
		selectedBGPPeers += selected
		skippedBGPPeersBySelector += skippedSelector
		skippedBGPPeersByLimit += skippedLimit
	}

	c.markEntitySelection(selectionEntityInterface, selectedInterfaces, skippedInterfacesBySelector, skippedInterfacesByLimit)
	c.markEntitySelection(selectionEntityBGPPeer, selectedBGPPeers, skippedBGPPeersBySelector, skippedBGPPeersByLimit)
}

func (c *Collector) pruneUnselectedSites(sites map[string]*siteState, order *[]string) {
	active := make(map[string]bool, len(c.discovery.siteIDs))
	nextOrder := make([]string, 0, len(c.discovery.siteIDs))
	for _, siteID := range c.discovery.siteIDs {
		if sites[siteID] == nil {
			continue
		}
		active[siteID] = true
		nextOrder = append(nextOrder, siteID)
	}
	for siteID := range sites {
		if !active[siteID] {
			delete(sites, siteID)
		}
	}
	*order = nextOrder
}

func (c *Collector) filterSiteInterfaces(site *siteState) (int, int, int) {
	if len(site.Interfaces) == 0 {
		return 0, 0, 0
	}

	limit := c.maxInterfacesPerSiteLimit()
	keys := sortedInterfaceKeys(site.Interfaces)
	next := make(map[string]*interfaceState, len(site.Interfaces))
	var selected, skippedSelector, skippedLimit int

	for _, key := range keys {
		iface := site.Interfaces[key]
		if iface == nil {
			continue
		}
		if !c.interfaceMatcher.matches(iface.ID, iface.Name) {
			skippedSelector++
			continue
		}
		if limit > 0 && selected >= limit {
			skippedLimit++
			continue
		}
		next[key] = iface
		selected++
	}

	site.Interfaces = next
	return selected, skippedSelector, skippedLimit
}

func (c *Collector) filterSiteBGPPeers(site *siteState) (int, int, int) {
	if len(site.BGPPeers) == 0 {
		return 0, 0, 0
	}

	limit := c.bgpMaxPeersPerSiteLimit()
	peers := append([]bgpPeerState(nil), site.BGPPeers...)
	sort.Slice(peers, func(i, j int) bool {
		return bgpPeerSortKey(peers[i]) < bgpPeerSortKey(peers[j])
	})

	next := make([]bgpPeerState, 0, len(peers))
	var selected, skippedSelector, skippedLimit int
	for _, peer := range peers {
		if !c.bgpPeerMatcher.matches(peer.RemoteIP, peer.RemoteASN) {
			skippedSelector++
			continue
		}
		if limit > 0 && selected >= limit {
			skippedLimit++
			continue
		}
		next = append(next, peer)
		selected++
	}

	site.BGPPeers = next
	return selected, skippedSelector, skippedLimit
}

func sortedInterfaceKeys(values map[string]*interfaceState) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left := values[keys[i]]
		right := values[keys[j]]
		return interfaceSortKey(left, keys[i]) < interfaceSortKey(right, keys[j])
	})
	return keys
}

func interfaceSortKey(iface *interfaceState, fallback string) string {
	if iface == nil {
		return fallback
	}
	return strings.Join([]string{iface.ID, iface.Name, fallback}, "\x00")
}

func bgpPeerSortKey(peer bgpPeerState) string {
	return strings.Join([]string{peer.RemoteIP, peer.RemoteASN, peer.LocalIP, peer.LocalASN}, "\x00")
}
