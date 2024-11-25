// SPDX-License-Identifier: GPL-3.0-or-later

package rabbitmq

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (r *RabbitMQ) collect() (map[string]int64, error) {
	if r.clusterName == "" {
		id, name, err := r.getClusterMeta()
		if err != nil {
			return nil, err
		}
		r.clusterId = id
		r.clusterName = name
	}

	r.cache.resetSeen()

	mx := make(map[string]int64)

	if err := r.collectOverview(mx); err != nil {
		return nil, err
	}
	if err := r.collectNodes(mx); err != nil {
		return mx, err
	}
	if err := r.collectVhosts(mx); err != nil {
		return mx, err
	}
	if r.CollectQueues {
		if err := r.collectQueues(mx); err != nil {
			return mx, err
		}
	}

	r.updateCharts()

	return mx, nil
}

func (r *RabbitMQ) getClusterMeta() (id string, name string, err error) {
	req, err := web.NewHTTPRequestWithPath(r.RequestConfig, urlPathAPIDefinitions)
	if err != nil {
		return "", "", fmt.Errorf("failed to create definitions request: %w", err)
	}

	var resp apiDefinitionsResp

	if err := r.webClient().RequestJSON(req, &resp); err != nil {
		return "", "", err
	}

	if resp.RabbitmqVersion == "" {
		return "", "", fmt.Errorf("unexpected response: rabbitmq version is empty")
	}

	id = "unknown"
	name = "unset"

	for _, v := range resp.GlobalParams {
		switch v.Name {
		case "cluster_name":
			name, _ = v.Value.(string)
		case "internal_cluster_id":
			id, _ = v.Value.(string)
			id = strings.TrimPrefix(id, "rabbitmq-cluster-id-")
		}
	}

	return id, name, nil
}

func (r *RabbitMQ) webClient() *web.Client {
	return web.DoHTTP(r.httpClient).OnNokCode(func(resp *http.Response) (bool, error) {
		var msg struct {
			Error  string `json:"error"`
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&msg); err == nil && msg.Error != "" {
			return false, fmt.Errorf("err '%s', reason '%s'", msg.Error, msg.Reason)
		}
		return false, nil
	})
}
