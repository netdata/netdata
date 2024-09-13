// SPDX-License-Identifier: GPL-3.0-or-later

package rabbitmq

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const (
	urlPathAPIOverview = "/api/overview"
	urlPathAPINodes    = "/api/nodes/"
	urlPathAPIVhosts   = "/api/vhosts"
	urlPathAPIQueues   = "/api/queues"
)

// TODO: there is built-in prometheus collector since v3.8.0 (https://www.rabbitmq.com/prometheus.html).
// Should use it (in addition?), it is the recommended option  according to the docs.
func (r *RabbitMQ) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := r.collectOverviewStats(mx); err != nil {
		return nil, err
	}
	if err := r.collectNodeStats(mx); err != nil {
		return mx, err
	}
	if err := r.collectVhostsStats(mx); err != nil {
		return mx, err
	}
	if r.CollectQueues {
		if err := r.collectQueuesStats(mx); err != nil {
			return mx, err
		}
	}

	return mx, nil
}

func (r *RabbitMQ) collectOverviewStats(mx map[string]int64) error {
	var stats overviewStats
	if err := r.doOKDecode(urlPathAPIOverview, &stats); err != nil {
		return err
	}

	if r.nodeName == "" {
		r.nodeName = stats.Node
	}

	for k, v := range stm.ToMap(stats) {
		mx[k] = v
	}

	return nil
}

func (r *RabbitMQ) collectNodeStats(mx map[string]int64) error {
	if r.nodeName == "" {
		return nil
	}

	var stats nodeStats
	if err := r.doOKDecode(filepath.Join(urlPathAPINodes, r.nodeName), &stats); err != nil {
		return err
	}

	for k, v := range stm.ToMap(stats) {
		mx[k] = v
	}
	mx["proc_available"] = int64(stats.ProcTotal - stats.ProcUsed)

	return nil
}

func (r *RabbitMQ) collectVhostsStats(mx map[string]int64) error {
	var stats []vhostStats
	if err := r.doOKDecode(urlPathAPIVhosts, &stats); err != nil {
		return err
	}

	seen := make(map[string]bool)

	for _, vhost := range stats {
		seen[vhost.Name] = true
		for k, v := range stm.ToMap(vhost) {
			mx[fmt.Sprintf("vhost_%s_%s", vhost.Name, k)] = v
		}
	}

	for name := range seen {
		if !r.vhosts[name] {
			r.vhosts[name] = true
			r.Debugf("new vhost name='%s': creating charts", name)
			r.addVhostCharts(name)
		}
	}
	for name := range r.vhosts {
		if !seen[name] {
			delete(r.vhosts, name)
			r.Debugf("stale vhost name='%s': removing charts", name)
			r.removeVhostCharts(name)
		}
	}

	return nil
}

func (r *RabbitMQ) collectQueuesStats(mx map[string]int64) error {
	var stats []queueStats
	if err := r.doOKDecode(urlPathAPIQueues, &stats); err != nil {
		return err
	}

	seen := make(map[string]queueCache)

	for _, queue := range stats {
		seen[queue.Name+"|"+queue.Vhost] = queueCache{name: queue.Name, vhost: queue.Vhost}
		for k, v := range stm.ToMap(queue) {
			mx[fmt.Sprintf("queue_%s_vhost_%s_%s", queue.Name, queue.Vhost, k)] = v
		}
	}

	for key, queue := range seen {
		if _, ok := r.queues[key]; !ok {
			r.queues[key] = queue
			r.Debugf("new queue name='%s', vhost='%s': creating charts", queue.name, queue.vhost)
			r.addQueueCharts(queue.name, queue.vhost)
		}
	}
	for key, queue := range r.queues {
		if _, ok := seen[key]; !ok {
			delete(r.queues, key)
			r.Debugf("stale queue name='%s', vhost='%s': removing charts", queue.name, queue.vhost)
			r.removeQueueCharts(queue.name, queue.vhost)
		}
	}

	return nil
}

func (r *RabbitMQ) doOKDecode(urlPath string, in interface{}) error {
	req, err := web.NewHTTPRequestWithPath(r.RequestConfig, urlPath)
	if err != nil {
		return fmt.Errorf("error on creating request: %v", err)
	}

	r.Debugf("doing HTTPConfig %s to '%s'", req.Method, req.URL)
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error on request to %s: %v", req.URL, err)
	}

	defer web.CloseBody(resp)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned HTTP status %d (%s)", req.URL, resp.StatusCode, resp.Status)
	}

	if err = json.NewDecoder(resp.Body).Decode(&in); err != nil {
		return fmt.Errorf("error on decoding response from %s: %v", req.URL, err)
	}

	return nil
}
