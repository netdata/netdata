// SPDX-License-Identifier: GPL-3.0-or-later

package pihole

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

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
		mx["blocking_status_enabled"] = boolToInt(pmx.summary.Status == "enabled")
		mx["blocking_status_disabled"] = boolToInt(pmx.summary.Status != "enabled")

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
	req, err := web.NewHTTPRequestWithPath(p.Request, urlPathAPI)
	if err != nil {
		p.Error(err)
		return
	}

	req.URL.RawQuery = url.Values{
		urlQueryKeyAuth:       []string{p.Password},
		urlQueryKeySummaryRaw: []string{"true"},
	}.Encode()

	var v summaryRawMetrics
	if err = p.doWithDecode(&v, req); err != nil {
		p.Error(err)
		return
	}

	pmx.summary = &v
}

func (p *Pihole) queryQueryTypes(pmx *piholeMetrics) {
	req, err := web.NewHTTPRequestWithPath(p.Request, urlPathAPI)
	if err != nil {
		p.Error(err)
		return
	}

	req.URL.RawQuery = url.Values{
		urlQueryKeyAuth:          []string{p.Password},
		urlQueryKeyGetQueryTypes: []string{"true"},
	}.Encode()

	var v queryTypesMetrics
	err = p.doWithDecode(&v, req)
	if err != nil {
		p.Error(err)
		return
	}

	pmx.queryTypes = &v
}

func (p *Pihole) queryForwardedDestinations(pmx *piholeMetrics) {
	req, err := web.NewHTTPRequestWithPath(p.Request, urlPathAPI)
	if err != nil {
		p.Error(err)
		return
	}

	req.URL.RawQuery = url.Values{
		urlQueryKeyAuth:                   []string{p.Password},
		urlQueryKeyGetForwardDestinations: []string{"true"},
	}.Encode()

	var v forwardDestinations
	err = p.doWithDecode(&v, req)
	if err != nil {
		p.Error(err)
		return
	}

	pmx.forwarders = &v
}

func (p *Pihole) queryAPIVersion() (int, error) {
	req, err := web.NewHTTPRequestWithPath(p.Request, urlPathAPI)
	if err != nil {
		return 0, err
	}

	req.URL.RawQuery = url.Values{
		urlQueryKeyAuth:       []string{p.Password},
		urlQueryKeyAPIVersion: []string{"true"},
	}.Encode()

	var v piholeAPIVersion
	err = p.doWithDecode(&v, req)
	if err != nil {
		return 0, err
	}

	return v.Version, nil
}

func (p *Pihole) doWithDecode(dst interface{}, req *http.Request) error {
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned %d status code", req.URL, resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error on reading response from %s : %v", req.URL, err)
	}

	// empty array if unauthorized query or wrong query
	if isEmptyArray(content) {
		return fmt.Errorf("unauthorized access to %s", req.URL)
	}

	if err := json.Unmarshal(content, dst); err != nil {
		return fmt.Errorf("error on parsing response from %s : %v", req.URL, err)
	}

	return nil
}

func isEmptyArray(data []byte) bool {
	empty := "[]"
	return len(data) == len(empty) && string(data) == empty
}

func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}

func boolToInt(b bool) int64 {
	if !b {
		return 0
	}
	return 1
}

func calcPercentage(value, total int64) (v int64) {
	if total == 0 {
		return 0
	}
	return int64(float64(value) * 100 / float64(total) * precision)
}
