// SPDX-License-Identifier: GPL-3.0-or-later

package pulsar

func newCache() *cache {
	return &cache{
		namespaces: make(map[namespace]bool),
		topics:     make(map[topic]bool),
	}
}

type (
	namespace struct{ name string }
	topic     struct{ namespace, name string }
	cache     struct {
		namespaces map[namespace]bool
		topics     map[topic]bool
	}
)
