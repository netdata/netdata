// SPDX-License-Identifier: GPL-3.0-or-later

package rabbitmq

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func (c *Collector) collect() (map[string]int64, error) {
	if c.queryClusterMeta {
		id, name, err := c.getClusterMeta()
		if err != nil {
			return nil, err
		}
		c.queryClusterMeta = false
		c.clusterId = id
		c.clusterName = name
	}

	c.cache.resetSeen()

	mx := make(map[string]int64)

	if err := c.collectOverview(mx); err != nil {
		return nil, err
	}
	if err := c.collectNodes(mx); err != nil {
		return mx, err
	}
	if err := c.collectVhosts(mx); err != nil {
		return mx, err
	}
	if c.CollectQueues {
		if err := c.collectQueues(mx); err != nil {
			return mx, err
		}
	}

	c.updateCharts()

	return mx, nil
}

func (c *Collector) getClusterMeta() (id string, name string, err error) {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathAPIWhoami)
	if err != nil {
		return "", "", fmt.Errorf("failed to create whoami request: %w", err)
	}

	var user apiWhoamiResp
	if err := c.webClient().RequestJSON(req, &user); err != nil {
		return "", "", fmt.Errorf("failed to send whoami request: %w", err)
	}

	if user.Name == "" {
		return "", "", fmt.Errorf("unexpected response: whoami: user name n is empty")
	}

	// In RabbitMQ < 3.8.3 the `tags` field may be returned as a single string
	// (e.g. "administrator,management") instead of an array. We intentionally
	// treat it as one tag and do not split on commas here.
	if !slices.ContainsFunc(user.Tags, func(s string) bool {
		return strings.Contains(s, "administrator")
	}) {
		c.Warningf("user %s lacks 'administrator' tag: cluster ID and name cannot be collected.", user.Name)
		return "", "", nil
	}

	req, err = web.NewHTTPRequestWithPath(c.RequestConfig, urlPathAPIDefinitions)
	if err != nil {
		return "", "", fmt.Errorf("failed to create definitions request: %w", err)
	}

	var resp apiDefinitionsResp

	if err := c.webClient().RequestJSON(req, &resp); err != nil {
		return "", "", err
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

func (c *Collector) webClient() *web.Client {
	return web.DoHTTP(c.httpClient).OnNokCode(func(resp *http.Response) (bool, error) {
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
