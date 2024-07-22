// SPDX-License-Identifier: GPL-3.0-or-later

package memcached

import (
	"fmt"
	"strconv"
	"strings"
)

type (
	responseMap map[string]int64
)

func (m *Memcached) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	stats, err := m.collectStats()
	if err != nil {
		return nil, err
	}

	mx["avail"] = stats["limit_maxbytes"] - stats["bytes"]
	mx["used"] = stats["bytes"]
	mx["bytes_read"] = stats["bytes_read"]
	mx["bytes_written"] = stats["bytes_written"]
	mx["curr_connections"] = stats["curr_connections"]
	mx["rejected_connections"] = stats["rejected_connections"]
	mx["total_connections"] = stats["total_connections"]
	mx["curr_items"] = stats["curr_items"]
	mx["total_items"] = stats["total_items"]
	mx["reclaimed"] = stats["reclaimed"]
	mx["evictions"] = stats["evictions"]
	mx["get_hits"] = stats["get_hits"]
	mx["get_misses"] = stats["get_misses"]
	mx["cmd_get"] = stats["cmd_get"]
	mx["cmd_set"] = stats["cmd_set"]
	mx["delete_hits"] = stats["delete_hits"]
	mx["delete_misses"] = stats["delete_misses"]
	mx["cas_hits"] = stats["cas_hits"]
	mx["cas_misses"] = stats["cas_misses"]
	mx["cas_badval"] = stats["cas_badval"]
	mx["incr_hits"] = stats["incr_hits"]
	mx["incr_misses"] = stats["incr_misses"]
	mx["decr_hits"] = stats["decr_hits"]
	mx["decr_misses"] = stats["decr_misses"]
	mx["touch_hits"] = stats["touch_hits"]
	mx["touch_misses"] = stats["touch_misses"]
	mx["cmd_touch"] = stats["cmd_touch"]

	return mx, nil
}

func (m *Memcached) collectStats() (responseMap, error) {
	conn := m.newMemcachedConn(m.Config)

	if err := conn.connect(); err != nil {
		return nil, err
	}
	defer conn.disconnect()

	msg, err := conn.queryStats()
	if err != nil {
		return nil, err
	}

	if len(msg) < 1 {
		return nil, fmt.Errorf("empty memcached response")
	}

	if !(strings.HasPrefix(msg, "STAT") && strings.HasSuffix(msg, "END")){
		return nil, fmt.Errorf("unexpected memcached response")
	}

	m.Debugf("memcached stats command response: %s", msg)

	parsed, err := parseResponse(msg)
	if err != nil {
		return nil, err
	}

	return *parsed, nil
}

func parseResponse(msg string) (*responseMap, error) {
	stats := make(responseMap)
	lines := strings.Split(msg, "STAT ")
	for _, line := range lines {
		if line == "" || strings.HasPrefix(line, "END") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 2 {
			key := fields[0]
			valueStr := fields[1]
			if intValue := parseInt(valueStr); intValue != nil {
				stats[key] = *intValue
			}
		}
	}
	return &stats, nil
}

func parseInt(value string) *int64 {
	v, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil
	}
	return &v
}
