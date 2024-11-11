// SPDX-License-Identifier: GPL-3.0-or-later

package pihole

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const wantAPIVersion = 3

const (
	urlPathAPI                        = "/admin/api.php"
	urlQueryKeyAuth                   = "auth"
	urlQueryKeyAPIVersion             = "version"
	urlQueryKeySummaryRaw             = "summaryRaw"
	urlQueryKeyGetQueryTypes          = "getQueryTypes"          // need auth
	urlQueryKeyGetForwardDestinations = "getForwardDestinations" // need auth
)

const (
	precision = 1000
)

func (p *Pihole) collect() (map[string]int64, error) {
	if p.checkVersion {
		ver, err := p.queryAPIVersion()
		if err != nil {
			return nil, err
		}
		if ver != wantAPIVersion {
			return nil, fmt.Errorf("API version: %d, supported version: %d", ver, wantAPIVersion)
		}
		p.checkVersion = false
	}

	pmx := new(piholeMetrics)
	p.queryMetrics(pmx, true)

	if pmx.hasQueryTypes() {
		p.addQueriesTypesOnce.Do(p.addChartDNSQueriesType)
	}
	if pmx.hasForwarders() {
		p.addFwsDestinationsOnce.Do(p.addChartDNSQueriesForwardedDestinations)
	}

	mx := make(map[string]int64)
	p.collectMetrics(mx, pmx)

	return mx, nil
}

func (p *Pihole) collectMetrics(mx map[string]int64, pmx *piholeMetrics) {
	if pmx.hasSummary() {
		mx["ads_blocked_today"] = pmx.summary.AdsBlockedToday
		mx["ads_percentage_today"] = int64(pmx.summary.AdsPercentageToday * 100)
		mx["domains_being_blocked"] = pmx.summary.DomainsBeingBlocked
		// GravityLastUpdated.Absolute is <nil> if the file does not exist (deleted/moved)
		if pmx.summary.GravityLastUpdated.Absolute != nil {
			mx["blocklist_last_update"] = time.Now().Unix() - *pmx.summary.GravityLastUpdated.Absolute
		}
		mx["dns_queries_today"] = pmx.summary.DNSQueriesToday
		mx["queries_forwarded"] = pmx.summary.QueriesForwarded
		mx["queries_cached"] = pmx.summary.QueriesCached
		mx["unique_clients"] = pmx.summary.UniqueClients
		mx["blocking_status_enabled"] = metrix.Bool(pmx.summary.Status == "enabled")
		mx["blocking_status_disabled"] = metrix.Bool(pmx.summary.Status != "enabled")

		tot := pmx.summary.QueriesCached + pmx.summary.AdsBlockedToday + pmx.summary.QueriesForwarded
		mx["queries_cached_perc"] = calcPercentage(pmx.summary.QueriesCached, tot)
		mx["ads_blocked_today_perc"] = calcPercentage(pmx.summary.AdsBlockedToday, tot)
		mx["queries_forwarded_perc"] = calcPercentage(pmx.summary.QueriesForwarded, tot)
	}

	if pmx.hasQueryTypes() {
		mx["A"] = int64(pmx.queryTypes.Types.A * 100)
		mx["AAAA"] = int64(pmx.queryTypes.Types.AAAA * 100)
		mx["ANY"] = int64(pmx.queryTypes.Types.ANY * 100)
		mx["PTR"] = int64(pmx.queryTypes.Types.PTR * 100)
		mx["SOA"] = int64(pmx.queryTypes.Types.SOA * 100)
		mx["SRV"] = int64(pmx.queryTypes.Types.SRV * 100)
		mx["TXT"] = int64(pmx.queryTypes.Types.TXT * 100)
	}

	if pmx.hasForwarders() {
		for k, v := range pmx.forwarders.Destinations {
			name := strings.Split(k, "|")[0]
			mx["destination_"+name] = int64(v * 100)
		}
	}
}

func (p *Pihole) queryMetrics(pmx *piholeMetrics, doConcurrently bool) {
	type task func(*piholeMetrics)

	var tasks = []task{p.querySummary}

	if p.Password != "" {
		tasks = []task{
			p.querySummary,
			p.queryQueryTypes,
			p.queryForwardedDestinations,
		}
	}

	wg := &sync.WaitGroup{}

	wrap := func(call task) task {
		return func(metrics *piholeMetrics) { call(metrics); wg.Done() }
	}

	for _, task := range tasks {
		if doConcurrently {
			wg.Add(1)
			task = wrap(task)
			go task(pmx)
		} else {
			task(pmx)
		}
	}

	wg.Wait()
}

func (p *Pihole) querySummary(pmx *piholeMetrics) {
	req, err := web.NewHTTPRequestWithPath(p.RequestConfig, urlPathAPI)
	if err != nil {
		p.Error(err)
		return
	}

	req.URL.RawQuery = url.Values{
		urlQueryKeyAuth:       []string{p.Password},
		urlQueryKeySummaryRaw: []string{"true"},
	}.Encode()

	var v summaryRawMetrics
	if err = p.doHTTP(req, &v); err != nil {
		p.Error(err)
		return
	}

	pmx.summary = &v
}

func (p *Pihole) queryQueryTypes(pmx *piholeMetrics) {
	req, err := web.NewHTTPRequestWithPath(p.RequestConfig, urlPathAPI)
	if err != nil {
		p.Error(err)
		return
	}

	req.URL.RawQuery = url.Values{
		urlQueryKeyAuth:          []string{p.Password},
		urlQueryKeyGetQueryTypes: []string{"true"},
	}.Encode()

	var v queryTypesMetrics
	err = p.doHTTP(req, &v)
	if err != nil {
		p.Error(err)
		return
	}

	pmx.queryTypes = &v
}

func (p *Pihole) queryForwardedDestinations(pmx *piholeMetrics) {
	req, err := web.NewHTTPRequestWithPath(p.RequestConfig, urlPathAPI)
	if err != nil {
		p.Error(err)
		return
	}

	req.URL.RawQuery = url.Values{
		urlQueryKeyAuth:                   []string{p.Password},
		urlQueryKeyGetForwardDestinations: []string{"true"},
	}.Encode()

	var v forwardDestinations
	err = p.doHTTP(req, &v)
	if err != nil {
		p.Error(err)
		return
	}

	pmx.forwarders = &v
}

func (p *Pihole) queryAPIVersion() (int, error) {
	req, err := web.NewHTTPRequestWithPath(p.RequestConfig, urlPathAPI)
	if err != nil {
		return 0, err
	}

	req.URL.RawQuery = url.Values{
		urlQueryKeyAuth:       []string{p.Password},
		urlQueryKeyAPIVersion: []string{"true"},
	}.Encode()

	var v piholeAPIVersion
	err = p.doHTTP(req, &v)
	if err != nil {
		return 0, err
	}

	return v.Version, nil
}

func (p *Pihole) doHTTP(req *http.Request, dst any) error {
	return web.DoHTTP(p.httpClient).Request(req, func(body io.Reader) error {
		content, err := io.ReadAll(body)
		if err != nil {
			return fmt.Errorf("failed to read response: %v", err)
		}

		// empty array if unauthorized query or wrong query
		if isEmptyArray(content) {
			return errors.New("unauthorized access")
		}

		if err := json.Unmarshal(content, dst); err != nil {
			return fmt.Errorf("failed to decode JSON response: %v", err)
		}

		return nil
	})
}

func isEmptyArray(data []byte) bool {
	empty := "[]"
	return len(data) == len(empty) && string(data) == empty
}

func calcPercentage(value, total int64) (v int64) {
	if total == 0 {
		return 0
	}
	return int64(float64(value) * 100 / float64(total) * precision)
}
