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

func (c *Collector) collect() (map[string]int64, error) {
	mx, err := c.collectServerStatus()
	if err != nil {
		return nil, err
	}

	return mx, nil
}

func (c *Collector) collectServerStatus() (map[string]int64, error) {
	resp, err := c.queryServerStatus()
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
		if !c.seenConnectors[v.Name] {
			c.seenConnectors[v.Name] = true
			c.addConnectorCharts(v.Name)
		}
	}

	for i, v := range resp.JVM.MemoryPools {
		resp.JVM.MemoryPools[i].STMKey = cleanName(v.Name)

		seenPools[v.Name] = true
		if !c.seenMemPools[v.Name] {
			c.seenMemPools[v.Name] = true
			c.addMemPoolCharts(v.Name, v.Type)
		}
	}

	for name := range c.seenConnectors {
		if !seenConns[name] {
			delete(c.seenConnectors, name)
			c.removeConnectorCharts(name)
		}
	}

	for name := range c.seenMemPools {
		if !seenPools[name] {
			delete(c.seenMemPools, name)
			c.removeMemoryPoolCharts(name)
		}
	}

	resp.JVM.Memory.Used = resp.JVM.Memory.Total - resp.JVM.Memory.Free

	return stm.ToMap(resp), nil
}

func cleanName(name string) string {
	r := strings.NewReplacer(" ", "_", ".", "_", "\"", "", "'", "")
	return strings.ToLower(r.Replace(name))
}

func (c *Collector) queryServerStatus() (*serverStatusResponse, error) {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathServerStatus)
	if err != nil {
		return nil, err
	}

	req.URL.RawQuery = urlQueryServerStatus

	var status serverStatusResponse
	if err := web.DoHTTP(c.httpClient).RequestXML(req, &status); err != nil {
		return nil, err
	}

	return &status, nil
}
