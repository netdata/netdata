// SPDX-License-Identifier: GPL-3.0-or-later

package confgroup

func NewCache() *Cache {
	return &Cache{
		hashes:  make(map[uint64]uint),
		sources: make(map[string]map[uint64]Config),
	}
}

type Cache struct {
	hashes  map[uint64]uint              // map[cfgHash]cfgCount
	sources map[string]map[uint64]Config // map[cfgSource]map[cfgHash]cfg
}

func (c *Cache) Add(group *Group) (added, removed []Config) {
	if group == nil {
		return nil, nil
	}

	if len(group.Configs) == 0 {
		return c.addEmpty(group)
	}

	return c.addNotEmpty(group)
}

func (c *Cache) addEmpty(group *Group) (added, removed []Config) {
	set, ok := c.sources[group.Source]
	if !ok {
		return nil, nil
	}

	for hash, cfg := range set {
		c.hashes[hash]--
		if c.hashes[hash] == 0 {
			removed = append(removed, cfg)
		}
		delete(set, hash)
	}

	delete(c.sources, group.Source)

	return nil, removed
}

func (c *Cache) addNotEmpty(group *Group) (added, removed []Config) {
	set, ok := c.sources[group.Source]
	if !ok {
		set = make(map[uint64]Config)
		c.sources[group.Source] = set
	}

	seen := make(map[uint64]struct{})

	for _, cfg := range group.Configs {
		hash := cfg.Hash()
		seen[hash] = struct{}{}

		if _, ok := set[hash]; ok {
			continue
		}

		set[hash] = cfg
		if c.hashes[hash] == 0 {
			added = append(added, cfg)
		}
		c.hashes[hash]++
	}

	if !ok {
		return added, nil
	}

	for hash, cfg := range set {
		if _, ok := seen[hash]; ok {
			continue
		}

		delete(set, hash)
		c.hashes[hash]--
		if c.hashes[hash] == 0 {
			removed = append(removed, cfg)
		}
	}

	if len(set) == 0 {
		delete(c.sources, group.Source)
	}

	return added, removed
}
