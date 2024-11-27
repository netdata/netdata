// SPDX-License-Identifier: GPL-3.0-or-later

package varnish

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
)

func (c *Collector) collect() (map[string]int64, error) {
	bs, err := c.exec.statistics()
	if err != nil {
		return nil, err
	}

	mx := make(map[string]int64)

	if err := c.collectStatistics(mx, bs); err != nil {
		return nil, err
	}

	return mx, nil
}

func (c *Collector) collectStatistics(mx map[string]int64, bs []byte) error {
	seenBackends, seenStorages := make(map[string]bool), make(map[string]bool)

	sc := bufio.NewScanner(bytes.NewReader(bs))

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 4 {
			c.Debugf("invalid line format: '%s'. Expected at least 4 fields, skipping line.", line)
			continue
		}

		fullMetric := parts[0]
		valueStr := parts[1]

		category, metric, ok := strings.Cut(fullMetric, ".")
		if !ok {
			c.Debugf("invalid metric format: '%s'. Expected 'category.metric', skipping metric.", fullMetric)
			continue
		}
		value, err := strconv.ParseInt(valueStr, 10, 64)
		if err != nil {
			c.Debugf("failed to parse metric '%s' value '%s': %v, skipping metric", fullMetric, valueStr, err)
			continue
		}

		switch category {
		case "MGT":
			if mgtMetrics[metric] {
				mx[fullMetric] = value
			}
		case "MAIN":
			if mainMetrics[metric] {
				mx[fullMetric] = value
			}
		case "SMA", "SMF", "MSE":
			storage, sMetric, ok := strings.Cut(metric, ".")
			if !ok {
				c.Debugf("invalid metric format: '%s'. Expected 'type.storage.metric', skipping metric.", fullMetric)
				continue
			}

			fullStorage := category + "." + storage

			if storageMetrics[sMetric] {
				seenStorages[fullStorage] = true
				mx[fullMetric] = value
			}
		case "VBE":
			// Varnish 4.0.x is not supported (values are 'VBE.default(127.0.0.1,,81).happy')
			parts := strings.Split(metric, ".")
			if len(parts) != 3 {
				c.Debugf("invalid metric format: '%s'. Expected 'VBE.vcl.backend.metric', skipping metric.", fullMetric)
				continue
			}

			vcl, backend, bMetric := parts[0], parts[1], parts[2]

			if backendMetrics[bMetric] {
				seenBackends[vcl+"."+backend] = true
				mx[fullMetric] = value
			}
		}
	}

	if len(mx) == 0 {
		return nil
	}

	for name := range seenStorages {
		if !c.seenStorages[name] {
			c.seenStorages[name] = true
			c.addStorageCharts(name)
		}
	}
	for name := range c.seenStorages {
		if !seenStorages[name] {
			delete(c.seenStorages, name)
			c.removeBackendCharts(name)
		}
	}

	for fullName := range seenBackends {
		if !c.seenBackends[fullName] {
			c.seenBackends[fullName] = true
			c.addBackendCharts(fullName)
		}
	}
	for fullName := range c.seenBackends {
		if !seenBackends[fullName] {
			delete(c.seenBackends, fullName)
			c.removeBackendCharts(fullName)
		}
	}

	return nil
}

var mgtMetrics = map[string]bool{
	"uptime":      true,
	"child_start": true,
	"child_exit":  true,
	"child_stop":  true,
	"child_died":  true,
	"child_dump":  true,
	"child_panic": true,
}

var mainMetrics = map[string]bool{
	"sess_conn":         true,
	"sess_dropped":      true,
	"client_req":        true,
	"cache_hit":         true,
	"cache_hitpass":     true,
	"cache_miss":        true,
	"cache_hitmiss":     true,
	"n_expired":         true,
	"n_lru_nuked":       true,
	"n_lru_moved":       true,
	"n_lru_limited":     true,
	"threads":           true,
	"threads_limited":   true,
	"threads_created":   true,
	"threads_destroyed": true,
	"threads_failed":    true,
	"thread_queue_len":  true,
	"backend_conn":      true,
	"backend_unhealthy": true,
	"backend_busy":      true,
	"backend_fail":      true,
	"backend_reuse":     true,
	"backend_recycle":   true,
	"backend_retry":     true,
	"backend_req":       true,
	"esi_errors":        true,
	"esi_warnings":      true,
	"uptime":            true,
}

var storageMetrics = map[string]bool{
	"g_space": true,
	"g_bytes": true,
	"g_alloc": true,
}

var backendMetrics = map[string]bool{
	"bereq_hdrbytes":   true,
	"bereq_bodybytes":  true,
	"beresp_hdrbytes":  true,
	"beresp_bodybytes": true,
}
