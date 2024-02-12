// SPDX-License-Identifier: GPL-3.0-or-later

package logstash

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/netdata/go.d.plugin/pkg/stm"
	"github.com/netdata/go.d.plugin/pkg/web"
)

const urlPathNodeStatsAPI = "/_node/stats"

func (l *Logstash) collect() (map[string]int64, error) {
	stats, err := l.queryNodeStats()
	if err != nil {
		return nil, err
	}

	l.updateCharts(stats.Pipelines)

	return stm.ToMap(stats), nil
}

func (l *Logstash) updateCharts(pipelines map[string]pipelineStats) {
	seen := make(map[string]bool)

	for id := range pipelines {
		seen[id] = true
		if !l.pipelines[id] {
			l.pipelines[id] = true
			l.addPipelineCharts(id)
		}
	}

	for id := range l.pipelines {
		if !seen[id] {
			delete(l.pipelines, id)
			l.removePipelineCharts(id)
		}
	}
}

func (l *Logstash) queryNodeStats() (*nodeStats, error) {
	req, _ := web.NewHTTPRequest(l.Request.Copy())
	req.URL.Path = urlPathNodeStatsAPI

	var stats nodeStats

	if err := l.doWithDecode(&stats, req); err != nil {
		return nil, err
	}

	return &stats, nil
}

func (l *Logstash) doWithDecode(dst interface{}, req *http.Request) error {
	l.Debugf("executing %s '%s'", req.Method, req.URL)
	resp, err := l.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned %d status code (%s)", req.URL, resp.StatusCode, resp.Status)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error on reading response from %s : %v", req.URL, err)
	}

	if err := json.Unmarshal(content, dst); err != nil {
		return fmt.Errorf("error on parsing response from %s : %v", req.URL, err)
	}

	return nil
}

func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}
