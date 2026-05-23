// SPDX-License-Identifier: GPL-3.0-or-later

package megacli

import (
	"strconv"
	"strings"
)

func (c *Collector) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := c.collectPhysDrives(mx); err != nil {
		return nil, err
	}

	if c.doBBU {
		if err := c.collectBBU(mx); err != nil {
			c.doBBU = false
			c.Warningf("BBU collection failed, disabling it: %v", err)
		}
	}

	return mx, nil
}

func writeInt(mx map[string]int64, key, value string) {
	v, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return
	}
	mx[key] = v
}

func getColonSepValue(line string) string {
	_, after, ok := strings.Cut(line, ":")
	if !ok {
		return ""
	}
	return strings.TrimSpace(after)
}

func getColonSepNumValue(line string) string {
	v := getColonSepValue(line)
	before, _, ok := strings.Cut(v, " ")
	if !ok {
		return v
	}
	return before
}
