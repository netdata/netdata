// SPDX-License-Identifier: GPL-3.0-or-later

package nginxvts

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (vts *NginxVTS) collect() (map[string]int64, error) {
	ms, err := vts.scapeVTS()
	if err != nil {
		return nil, nil
	}

	collected := make(map[string]interface{})
	vts.collectMain(collected, ms)
	vts.collectSharedZones(collected, ms)
	vts.collectServerZones(collected, ms)

	return stm.ToMap(collected), nil
}

func (vts *NginxVTS) collectMain(collected map[string]interface{}, ms *vtsMetrics) {
	collected["uptime"] = (ms.NowMsec - ms.LoadMsec) / 1000
	collected["connections"] = ms.Connections
}

func (vts *NginxVTS) collectSharedZones(collected map[string]interface{}, ms *vtsMetrics) {
	collected["sharedzones"] = ms.SharedZones
}

func (vts *NginxVTS) collectServerZones(collected map[string]interface{}, ms *vtsMetrics) {
	if !ms.hasServerZones() {
		return
	}

	// "*" means all servers
	collected["total"] = ms.ServerZones["*"]
}

func (vts *NginxVTS) scapeVTS() (*vtsMetrics, error) {
	req, _ := web.NewHTTPRequest(vts.RequestConfig)

	var total vtsMetrics

	if err := vts.doOKDecode(req, &total); err != nil {
		vts.Warning(err)
		return nil, err
	}
	return &total, nil
}

func (vts *NginxVTS) doOKDecode(req *http.Request, in interface{}) error {
	resp, err := vts.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error on HTTPConfig request '%s': %v", req.URL, err)
	}

	defer web.CloseBody(resp)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("'%s' returned HTTP status code: %d", req.URL, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(in); err != nil {
		return fmt.Errorf("error on decoding response from '%s': %v", req.URL, err)
	}
	return nil
}
