// SPDX-License-Identifier: GPL-3.0-or-later

package confgroup

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigCache_Add(t *testing.T) {
	tests := map[string]struct {
		prepareGroups  []Group
		groups         []Group
		expectedAdd    []Config
		expectedRemove []Config
	}{
		"new group, new configs": {
			groups: []Group{
				prepareGroup("source", prepareCfg("name", "module")),
			},
			expectedAdd: []Config{
				prepareCfg("name", "module"),
			},
		},
		"several equal updates for the same group": {
			groups: []Group{
				prepareGroup("source", prepareCfg("name", "module")),
				prepareGroup("source", prepareCfg("name", "module")),
				prepareGroup("source", prepareCfg("name", "module")),
				prepareGroup("source", prepareCfg("name", "module")),
				prepareGroup("source", prepareCfg("name", "module")),
			},
			expectedAdd: []Config{
				prepareCfg("name", "module"),
			},
		},
		"empty group update for cached group": {
			prepareGroups: []Group{
				prepareGroup("source", prepareCfg("name1", "module"), prepareCfg("name2", "module")),
			},
			groups: []Group{
				prepareGroup("source"),
			},
			expectedRemove: []Config{
				prepareCfg("name1", "module"),
				prepareCfg("name2", "module"),
			},
		},
		"changed group update for cached group": {
			prepareGroups: []Group{
				prepareGroup("source", prepareCfg("name1", "module"), prepareCfg("name2", "module")),
			},
			groups: []Group{
				prepareGroup("source", prepareCfg("name2", "module")),
			},
			expectedRemove: []Config{
				prepareCfg("name1", "module"),
			},
		},
		"empty group update for uncached group": {
			groups: []Group{
				prepareGroup("source"),
				prepareGroup("source"),
			},
		},
		"several updates with different source but same context": {
			groups: []Group{
				prepareGroup("source1", prepareCfg("name1", "module"), prepareCfg("name2", "module")),
				prepareGroup("source2", prepareCfg("name1", "module"), prepareCfg("name2", "module")),
			},
			expectedAdd: []Config{
				prepareCfg("name1", "module"),
				prepareCfg("name2", "module"),
			},
		},
		"have equal configs from 2 sources, get empty group for the 1st source": {
			prepareGroups: []Group{
				prepareGroup("source1", prepareCfg("name1", "module"), prepareCfg("name2", "module")),
				prepareGroup("source2", prepareCfg("name1", "module"), prepareCfg("name2", "module")),
			},
			groups: []Group{
				prepareGroup("source2"),
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cache := NewCache()

			for _, group := range test.prepareGroups {
				cache.Add(&group)
			}

			var added, removed []Config
			for _, group := range test.groups {
				a, r := cache.Add(&group)
				added = append(added, a...)
				removed = append(removed, r...)
			}

			sortConfigs(added)
			sortConfigs(removed)
			sortConfigs(test.expectedAdd)
			sortConfigs(test.expectedRemove)

			assert.Equalf(t, test.expectedAdd, added, "added configs")
			assert.Equalf(t, test.expectedRemove, removed, "removed configs")
		})
	}
}

func prepareGroup(source string, cfgs ...Config) Group {
	return Group{
		Configs: cfgs,
		Source:  source,
	}
}

func prepareCfg(name, module string) Config {
	return Config{
		"name":   name,
		"module": module,
	}
}

func sortConfigs(cfgs []Config) {
	if len(cfgs) == 0 {
		return
	}
	sort.Slice(cfgs, func(i, j int) bool { return cfgs[i].FullName() < cfgs[j].FullName() })
}
