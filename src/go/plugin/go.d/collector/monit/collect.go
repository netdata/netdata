// SPDX-License-Identifier: GPL-3.0-or-later

package monit

import (
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"

	"golang.org/x/net/html/charset"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

var (
	urlPathStatus  = "/_status"
	urlQueryStatus = url.Values{"format": {"xml"}, "level": {"full"}}.Encode()
)

func (c *Collector) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := c.collectStatus(mx); err != nil {
		return nil, err
	}

	return mx, nil
}

func (c *Collector) collectStatus(mx map[string]int64) error {
	status, err := c.fetchStatus()
	if err != nil {
		return err
	}

	if status.Server == nil {
		// not Monit
		return errors.New("invalid Collector status response: missing server data")
	}

	mx["uptime"] = status.Server.Uptime

	seen := make(map[string]bool)

	for _, svc := range status.Services {
		seen[svc.id()] = true

		if _, ok := c.seenServices[svc.id()]; !ok {
			c.seenServices[svc.id()] = svc
			c.addServiceCheckCharts(svc, status.Server)
		}

		px := fmt.Sprintf("service_check_type_%s_name_%s_status_", svc.svcType(), svc.Name)

		for _, v := range []string{"not_monitored", "ok", "initializing", "error"} {
			mx[px+v] = 0
			if svc.status() == v {
				mx[px+v] = 1
			}
		}
	}

	for id, svc := range c.seenServices {
		if !seen[id] {
			delete(c.seenServices, id)
			c.removeServiceCharts(svc)
		}
	}

	return nil
}

func (c *Collector) fetchStatus() (*monitStatus, error) {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathStatus)
	if err != nil {
		return nil, err
	}
	req.URL.RawQuery = urlQueryStatus

	var status monitStatus
	if err := web.DoHTTP(c.httpClient).RequestXML(req, &status, func(d *xml.Decoder) {
		d.CharsetReader = charset.NewReaderLabel
	}); err != nil {
		return nil, err
	}

	return &status, nil
}
