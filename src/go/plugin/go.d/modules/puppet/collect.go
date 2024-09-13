// SPDX-License-Identifier: GPL-3.0-or-later

package puppet

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

var (
	//https://puppet.com/docs/puppet/8/server/status-api/v1/services
	urlPathStatusService  = "/status/v1/services"
	urlQueryStatusService = url.Values{"level": {"debug"}}.Encode()
)

func (p *Puppet) collect() (map[string]int64, error) {
	stats, err := p.queryStatsService()
	if err != nil {
		return nil, err
	}

	mx := stm.ToMap(stats)

	return mx, nil
}

func (p *Puppet) queryStatsService() (*statusServiceResponse, error) {
	req, err := web.NewHTTPRequestWithPath(p.RequestConfig, urlPathStatusService)
	if err != nil {
		return nil, err
	}

	req.URL.RawQuery = urlQueryStatusService

	var stats statusServiceResponse
	if err := p.doOKDecode(req, &stats); err != nil {
		return nil, err
	}

	if stats.StatusService == nil {
		return nil, fmt.Errorf("unexpected response: not puppet service status data")
	}

	return &stats, nil
}

func (p *Puppet) doOKDecode(req *http.Request, in interface{}) error {
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error on HTTPConfig request '%s': %v", req.URL, err)
	}

	defer web.CloseBody(resp)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("'%s' returned HTTPConfig status code: %d", req.URL, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(in); err != nil {
		return fmt.Errorf("error on decoding response from '%s': %v", req.URL, err)
	}
	return nil
}
