// SPDX-License-Identifier: GPL-3.0-or-later

package filecheck

func newSeenItems() *seenItems {
	return &seenItems{
		items: make(map[string]*seenItem),
	}
}

type (
	seenItems struct {
		items map[string]*seenItem
	}
	seenItem struct {
		hasExistenceCharts bool
		hasOtherCharts     bool
	}
)

func (c *seenItems) getp(path string) *seenItem {
	item, ok := c.items[path]
	if !ok {
		item = &seenItem{}
		c.items[path] = item
	}
	return item
}
