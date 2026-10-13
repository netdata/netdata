// SPDX-License-Identifier: GPL-3.0-or-later

package adaptecraid

import (
	"strings"
)

func (c *Collector) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := c.collectLogicalDevices(mx); err != nil {
		return nil, err
	}
	if err := c.collectPhysicalDevices(mx); err != nil {
		return nil, err
	}

	return mx, nil
}

func getColonSepValue(line string) string {
	_, after, ok := strings.Cut(line, ":")
	if !ok {
		return ""
	}
	return strings.TrimSpace(after)
}
