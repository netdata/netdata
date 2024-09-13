// SPDX-License-Identifier: GPL-3.0-or-later

package monit

import (
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"

	"golang.org/x/net/html/charset"
)

var (
	urlPathStatus  = "/_status"
	urlQueryStatus = url.Values{"format": {"xml"}, "level": {"full"}}.Encode()
)

func (m *Monit) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := m.collectStatus(mx); err != nil {
		return nil, err
	}

	return mx, nil
}

func (m *Monit) collectStatus(mx map[string]int64) error {
	status, err := m.fetchStatus()
	if err != nil {
		return err
	}

	if status.Server == nil {
		// not Monit
		return errors.New("invalid Monit status response: missing server data")
	}

	mx["uptime"] = status.Server.Uptime

	seen := make(map[string]bool)

	for _, svc := range status.Services {
		seen[svc.id()] = true

		if _, ok := m.seenServices[svc.id()]; !ok {
			m.seenServices[svc.id()] = svc
			m.addServiceCheckCharts(svc, status.Server)
		}

		px := fmt.Sprintf("service_check_type_%s_name_%s_status_", svc.svcType(), svc.Name)

		for _, v := range []string{"not_monitored", "ok", "initializing", "error"} {
			mx[px+v] = 0
			if svc.status() == v {
				mx[px+v] = 1
			}
		}
	}

	for id, svc := range m.seenServices {
		if !seen[id] {
			delete(m.seenServices, id)
			m.removeServiceCharts(svc)
		}
	}

	return nil
}

func (m *Monit) fetchStatus() (*monitStatus, error) {
	req, err := web.NewHTTPRequestWithPath(m.Request, urlPathStatus)
	if err != nil {
		return nil, err
	}
	req.URL.RawQuery = urlQueryStatus

	var status monitStatus
	if err := m.doOKDecode(req, &status); err != nil {
		return nil, err
	}

	return &status, nil
}

func (m *Monit) doOKDecode(req *http.Request, in interface{}) error {
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error on HTTP request '%s': %v", req.URL, err)
	}

	defer web.CloseBody(resp)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("'%s' returned HTTP status code: %d", req.URL, resp.StatusCode)
	}

	dec := xml.NewDecoder(resp.Body)
	dec.CharsetReader = charset.NewReaderLabel

	if err := dec.Decode(in); err != nil {
		return fmt.Errorf("error on decoding XML response from '%s': %v", req.URL, err)
	}

	return nil
}
