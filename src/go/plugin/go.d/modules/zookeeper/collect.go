// SPDX-License-Identifier: GPL-3.0-or-later

package zookeeper

import (
	"fmt"
	"strconv"
	"strings"
)

func (z *Zookeeper) collect() (map[string]int64, error) {
	return z.collectMntr()
}

func (z *Zookeeper) collectMntr() (map[string]int64, error) {
	const command = "mntr"

	lines, err := z.fetch("mntr")
	if err != nil {
		return nil, err
	}

	switch len(lines) {
	case 0:
		return nil, fmt.Errorf("'%s' command returned empty response", command)
	case 1:
		// mntr is not executed because it is not in the whitelist.
		return nil, fmt.Errorf("'%s' command returned bad response: %s", command, lines[0])
	}

	mx := make(map[string]int64)

	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) != 2 || !strings.HasPrefix(parts[0], "zk_") {
			continue
		}

		key, value := strings.TrimPrefix(parts[0], "zk_"), parts[1]
		switch key {
		case "version":
		case "server_state":
			mx[key] = convertServerState(value)
		case "min_latency", "avg_latency", "max_latency":
			v, err := strconv.ParseFloat(value, 64)
			if err != nil {
				continue
			}
			mx[key] = int64(v * 1000)
		default:
			v, err := strconv.ParseFloat(value, 64)
			if err != nil {
				continue
			}
			mx[key] = int64(v)
		}
	}

	if len(mx) == 0 {
		return nil, fmt.Errorf("'%s' command: failed to parse response", command)
	}

	return mx, nil
}

func convertServerState(state string) int64 {
	switch state {
	default:
		return 0
	case "leader":
		return 1
	case "follower":
		return 2
	case "observer":
		return 3
	case "standalone":
		return 4
	}
}
