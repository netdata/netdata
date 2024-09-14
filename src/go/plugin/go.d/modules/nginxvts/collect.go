// SPDX-License-Identifier: GPL-3.0-or-later

package nginxvts

import (
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
	if err := web.DoHTTP(vts.httpClient).RequestJSON(req, &total); err != nil {
		vts.Warning(err)
		return nil, err
	}

	return &total, nil
}
