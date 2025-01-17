// SPDX-License-Identifier: GPL-3.0-or-later

package ethtool

import (
	"strings"
)

func (c *Collector) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	for _, iface := range strings.Fields(c.OpticInterfaces) {
		if c.ignoredOpticIfaces[iface] {
			continue
		}

		if err := c.collectModuleEeprom(mx, iface); err != nil {
			c.ignoredOpticIfaces[iface] = true
			c.Errorf("failed to collect ddm info for %s: %v", iface, err)
		}
	}

	return mx, nil
}
