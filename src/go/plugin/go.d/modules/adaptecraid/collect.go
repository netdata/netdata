// SPDX-License-Identifier: GPL-3.0-or-later

package adaptecraid

import (
	"strings"
)

func (a *AdaptecRaid) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := a.collectLogicalDevices(mx); err != nil {
		return nil, err
	}
	if err := a.collectPhysicalDevices(mx); err != nil {
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
