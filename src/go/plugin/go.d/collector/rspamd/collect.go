// SPDX-License-Identifier: GPL-3.0-or-later

package rspamd

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

type rspamdStats struct {
	Version  string `json:"version"`
	ConfigId string `json:"config_id"`
	Scanned  *int64 `json:"scanned" stm:"scanned"`
	Learned  *int64 `json:"learned" stm:"learned"`
	Actions  struct {
		Reject           int64 `json:"reject" stm:"reject"`
		SoftReject       int64 `json:"soft reject" stm:"soft_reject"`
		RewriteSubject   int64 `json:"rewrite subject" stm:"rewrite_subject"`
		AddHeader        int64 `json:"add header" stm:"add_header"`
		Greylist         int64 `json:"greylist" stm:"greylist"`
		NoAction         int64 `json:"no action" stm:"no_action"`
		InvalidMaxAction int64 `json:"invalid max action" stm:"invalid_max_action"`
		Custom           int64 `json:"custom" stm:"custom"`
		Discard          int64 `json:"discard" stm:"discard"`
		Quarantine       int64 `json:"quarantine" stm:"quarantine"`
		UnknownAction    int64 `json:"unknown action" stm:"unknown_action"`
	} `json:"actions" stm:"actions"`
	ScanTimes          []float64        `json:"scan_times"`
	SpamCount          int64            `json:"spam_count" stm:"spam_count"`
	HamCount           int64            `json:"ham_count" stm:"ham_count"`
	Connections        int64            `json:"connections" stm:"connections"`
	ControlConnections int64            `json:"control_connections" stm:"control_connections"`
	FuzzyHashes        map[string]int64 `json:"fuzzy_hashes"`
}

func (c *Collector) collect() (map[string]int64, error) {
	stats, err := c.queryRspamdStats()
	if err != nil {
		return nil, err
	}

	mx := stm.ToMap(stats)

	return mx, nil
}

func (c *Collector) queryRspamdStats() (*rspamdStats, error) {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, "/stat")
	if err != nil {
		return nil, err
	}

	var stats rspamdStats
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &stats); err != nil {
		return nil, err
	}

	if stats.Scanned == nil || stats.Learned == nil {
		return nil, fmt.Errorf("unexpected response: not rspamd data")
	}

	return &stats, nil
}
