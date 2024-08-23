// SPDX-License-Identifier: GPL-3.0-or-later

package rspamd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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

func (r *Rspamd) collect() (map[string]int64, error) {
	stats, err := r.queryRspamdStats()
	if err != nil {
		return nil, err
	}

	mx := stm.ToMap(stats)

	return mx, nil
}

func (r *Rspamd) queryRspamdStats() (*rspamdStats, error) {
	req, err := web.NewHTTPRequestWithPath(r.Request, "/stat")
	if err != nil {
		return nil, err
	}

	var stats rspamdStats
	if err := r.doOKDecode(req, &stats); err != nil {
		return nil, err
	}

	if stats.Scanned == nil || stats.Learned == nil {
		return nil, fmt.Errorf("unexpected response: not rspamd data")
	}

	return &stats, nil
}

func (r *Rspamd) doOKDecode(req *http.Request, in interface{}) error {
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error on HTTP request '%s': %v", req.URL, err)
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("'%s' returned HTTP status code: %d", req.URL, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(in); err != nil {
		return fmt.Errorf("error on decoding response from '%s': %v", req.URL, err)
	}
	return nil
}

func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}
