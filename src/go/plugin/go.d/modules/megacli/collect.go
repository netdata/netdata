// SPDX-License-Identifier: GPL-3.0-or-later

package megacli

import (
	"strconv"
	"strings"
)

func (m *MegaCli) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := m.collectPhysDrives(mx); err != nil {
		return nil, err
	}
	if err := m.collectBBU(mx); err != nil {
		return nil, err
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
	i := strings.IndexByte(line, ':')
	if i == -1 {
		return ""
	}
	return strings.TrimSpace(line[i+1:])
}

func getColonSepNumValue(line string) string {
	v := getColonSepValue(line)
	i := strings.IndexByte(v, ' ')
	if i == -1 {
		return v
	}
	return v[:i]
}
