// SPDX-License-Identifier: GPL-3.0-or-later

package nginxvts

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/netdata/go.d.plugin/pkg/stm"
	"github.com/netdata/go.d.plugin/pkg/web"
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
	req, _ := web.NewHTTPRequest(vts.Request)

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
