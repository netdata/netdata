// SPDX-License-Identifier: GPL-3.0-or-later

package tomcat

import (
	"errors"
	"net/url"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

var (
	urlPathServerStatus  = "/manager/status"
	urlQueryServerStatus = url.Values{"XML": {"true"}}.Encode()
)

func (t *Tomcat) collect() (map[string]int64, error) {
	mx, err := t.collectServerStatus()
	if err != nil {
		return nil, err
	}

	return mx, nil
}

func (t *Tomcat) collectServerStatus() (map[string]int64, error) {
	resp, err := t.queryServerStatus()
	if err != nil {
		return nil, err
	}

	if len(resp.Connectors) == 0 {
		return nil, errors.New("unexpected response: not tomcat server status data")
	}

	seenConns, seenPools := make(map[string]bool), make(map[string]bool)

	for i, v := range resp.Connectors {
		resp.Connectors[i].STMKey = cleanName(v.Name)
		ti := &resp.Connectors[i].ThreadInfo
		ti.CurrentThreadsIdle = ti.CurrentThreadCount - ti.CurrentThreadsBusy

		seenConns[v.Name] = true
		if !t.seenConnectors[v.Name] {
			t.seenConnectors[v.Name] = true
			t.addConnectorCharts(v.Name)
		}
	}

	for i, v := range resp.JVM.MemoryPools {
		resp.JVM.MemoryPools[i].STMKey = cleanName(v.Name)

		seenPools[v.Name] = true
		if !t.seenMemPools[v.Name] {
			t.seenMemPools[v.Name] = true
			t.addMemPoolCharts(v.Name, v.Type)
		}
	}

	for name := range t.seenConnectors {
		if !seenConns[name] {
			delete(t.seenConnectors, name)
			t.removeConnectorCharts(name)
		}
	}

	for name := range t.seenMemPools {
		if !seenPools[name] {
			delete(t.seenMemPools, name)
			t.removeMemoryPoolCharts(name)
		}
	}

	resp.JVM.Memory.Used = resp.JVM.Memory.Total - resp.JVM.Memory.Free

	return stm.ToMap(resp), nil
}

func cleanName(name string) string {
	r := strings.NewReplacer(" ", "_", ".", "_", "\"", "", "'", "")
	return strings.ToLower(r.Replace(name))
}

func (t *Tomcat) queryServerStatus() (*serverStatusResponse, error) {
	req, err := web.NewHTTPRequestWithPath(t.RequestConfig, urlPathServerStatus)
	if err != nil {
		return nil, err
	}

	req.URL.RawQuery = urlQueryServerStatus

	var status serverStatusResponse
	if err := web.DoHTTP(t.httpClient).RequestXML(req, &status); err != nil {
		return nil, err
	}

	return &status, nil
}
