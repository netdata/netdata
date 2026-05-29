// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
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
	return nil
}

func newEntitySelector(name, expr string) (*entitySelector, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" || expr == "*" {
		return nil, nil
	}
	selector := &entitySelector{}
	for term := range strings.FieldsSeq(expr) {
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

func (c *Collector) selectSites(siteIDs []string, siteNames map[string]string) ([]string, int) {
	selected := make([]string, 0, len(siteIDs))
	var skippedSelector int

	for _, siteID := range siteIDs {
		if !c.siteMatcher.matches(siteID, siteNames[siteID]) {
			skippedSelector++
			continue
		}
		selected = append(selected, siteID)
	}

	return selected, skippedSelector
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
