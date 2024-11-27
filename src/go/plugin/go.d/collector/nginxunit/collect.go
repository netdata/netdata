// SPDX-License-Identifier: GPL-3.0-or-later

package nginxunit

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const (
	urlPathStatus = "/status"
)

// https://unit.nginx.org/statusapi/
type nuStatus struct {
	Connections *struct {
		Accepted int64 `json:"accepted" stm:"accepted"`
		Active   int64 `json:"active" stm:"active"`
		Idle     int64 `json:"idle" stm:"idle"`
		Closed   int64 `json:"closed" stm:"closed"`
	} `json:"connections" stm:"connections"`
	Requests struct {
		Total int64 `json:"total" stm:"total"`
	} `json:"requests" stm:"requests"`
}

func (c *Collector) collect() (map[string]int64, error) {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathStatus)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request to '%s': %v", c.URL, err)
	}

	var status nuStatus

	wc := web.DoHTTP(c.httpClient).OnNokCode(func(resp *http.Response) (bool, error) {
		var msg struct {
			Error string `json:"error"`
		}
		if json.NewDecoder(resp.Body).Decode(&msg) == nil && msg.Error != "" {
			return false, errors.New(msg.Error)
		}
		return false, nil
	})

	if err := wc.RequestJSON(req, &status); err != nil {
		return nil, err
	}

	if status.Connections == nil {
		return nil, errors.New("unexpected response: no connections available")
	}

	return stm.ToMap(status), nil
}
