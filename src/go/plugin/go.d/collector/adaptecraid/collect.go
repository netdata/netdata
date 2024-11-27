// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

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
	i := strings.IndexByte(line, ':')
	if i == -1 {
		return ""
	}
	return strings.TrimSpace(line[i+1:])
}
