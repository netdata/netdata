// SPDX-License-Identifier: GPL-3.0-or-later

package traefik

import (
	"errors"
	"strings"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/pkg/prometheus"
)

const (
	metricEntrypointRequestsTotal               = "traefik_entrypoint_requests_total"
	metricEntrypointRequestDurationSecondsSum   = "traefik_entrypoint_request_duration_seconds_sum"
	metricEntrypointRequestDurationSecondsCount = "traefik_entrypoint_request_duration_seconds_count"
	metricEntrypointOpenConnections             = "traefik_entrypoint_open_connections"
)

const (
	prefixEntrypointRequests  = "entrypoint_requests_"
	prefixEntrypointReqDurAvg = "entrypoint_request_duration_average_"
	prefixEntrypointOpenConn  = "entrypoint_open_connections_"
)

func isTraefikMetrics(pms prometheus.Series) bool {
	for _, pm := range pms {
		if strings.HasPrefix(pm.Name(), "traefik_") {
			return true
		}
	}
	return false
}

func (t *Traefik) collect() (map[string]int64, error) {
	pms, err := t.prom.ScrapeSeries()
	if err != nil {
		return nil, err
	}

	if t.checkMetrics && !isTraefikMetrics(pms) {
		return nil, errors.New("unexpected metrics (not Traefik)")
	}
	t.checkMetrics = false

	mx := make(map[string]int64)

	t.collectEntrypointRequestsTotal(mx, pms)
	t.collectEntrypointRequestDuration(mx, pms)
	t.collectEntrypointOpenConnections(mx, pms)
	t.updateCodeClassMetrics(mx)

	return mx, nil
}

func (t *Traefik) collectEntrypointRequestsTotal(mx map[string]int64, pms prometheus.Series) {
	if pms = pms.FindByName(metricEntrypointRequestsTotal); pms.Len() == 0 {
		return
	}

	for _, pm := range pms {
		code := pm.Labels.Get("code")
		ep := pm.Labels.Get("entrypoint")
		proto := pm.Labels.Get("protocol")
		codeClass := getCodeClass(code)
		if code == "" || ep == "" || proto == "" || codeClass == "" {
			continue
		}

		key := prefixEntrypointRequests + ep + "_" + proto + "_" + codeClass
		mx[key] += int64(pm.Value)

		id := ep + "_" + proto
		ce := t.cacheGetOrPutEntrypoint(id)
		if ce.requests == nil {
			chart := newChartEntrypointRequests(ep, proto)
			ce.requests = chart
			if err := t.Charts().Add(chart); err != nil {
				t.Warning(err)
			}
		}
	}
}

func (t *Traefik) collectEntrypointRequestDuration(mx map[string]int64, pms prometheus.Series) {
	if pms = pms.FindByNames(
		metricEntrypointRequestDurationSecondsCount,
		metricEntrypointRequestDurationSecondsSum,
	); pms.Len() == 0 {
		return
	}

	for _, pm := range pms {
		code := pm.Labels.Get("code")
		ep := pm.Labels.Get("entrypoint")
		proto := pm.Labels.Get("protocol")
		codeClass := getCodeClass(code)
		if code == "" || ep == "" || proto == "" || codeClass == "" {
			continue
		}

		id := ep + "_" + proto
		ce := t.cacheGetOrPutEntrypoint(id)
		v := ce.reqDurData[codeClass]
		if pm.Name() == metricEntrypointRequestDurationSecondsSum {
			v.cur.secs += pm.Value
		} else {
			v.cur.reqs += pm.Value
		}
		ce.reqDurData[codeClass] = v
	}

	for id, ce := range t.cache.entrypoints {
		if ce.reqDur == nil {
			chart := newChartEntrypointRequestDuration(ce.name, ce.proto)
			ce.reqDur = chart
			if err := t.Charts().Add(chart); err != nil {
				t.Warning(err)
			}
		}
		for codeClass, v := range ce.reqDurData {
			secs, reqs, seen := v.cur.secs-v.prev.secs, v.cur.reqs-v.prev.reqs, v.seen
			v.prev.secs, v.prev.reqs, v.seen = v.cur.secs, v.cur.reqs, true
			v.cur.secs, v.cur.reqs = 0, 0
			ce.reqDurData[codeClass] = v

			key := prefixEntrypointReqDurAvg + id + "_" + codeClass
			if secs <= 0 || reqs <= 0 || !seen {
				mx[key] = 0
			} else {
				mx[key] = int64(secs * 1000 / reqs)
			}
		}
	}
}

func (t *Traefik) collectEntrypointOpenConnections(mx map[string]int64, pms prometheus.Series) {
	if pms = pms.FindByName(metricEntrypointOpenConnections); pms.Len() == 0 {
		return
	}

	for _, pm := range pms {
		method := pm.Labels.Get("method")
		ep := pm.Labels.Get("entrypoint")
		proto := pm.Labels.Get("protocol")
		if method == "" || ep == "" || proto == "" {
			continue
		}

		key := prefixEntrypointOpenConn + ep + "_" + proto + "_" + method
		mx[key] += int64(pm.Value)

		id := ep + "_" + proto
		ce := t.cacheGetOrPutEntrypoint(id)
		if ce.openConn == nil {
			chart := newChartEntrypointOpenConnections(ep, proto)
			ce.openConn = chart
			if err := t.Charts().Add(chart); err != nil {
				t.Warning(err)
			}
		}

		if !ce.openConnMethods[method] {
			ce.openConnMethods[method] = true
			dim := &module.Dim{ID: key, Name: method}
			if err := ce.openConn.AddDim(dim); err != nil {
				t.Warning(err)
			}
		}
	}
}

var httpRespCodeClasses = []string{"1xx", "2xx", "3xx", "4xx", "5xx"}

func (t Traefik) updateCodeClassMetrics(mx map[string]int64) {
	for id, ce := range t.cache.entrypoints {
		if ce.requests != nil {
			for _, c := range httpRespCodeClasses {
				key := prefixEntrypointRequests + id + "_" + c
				mx[key] += 0
			}
		}
		if ce.reqDur != nil {
			for _, c := range httpRespCodeClasses {
				key := prefixEntrypointReqDurAvg + id + "_" + c
				mx[key] += 0
			}
		}
	}
}

func getCodeClass(code string) string {
	if len(code) != 3 {
		return ""
	}
	return string(code[0]) + "xx"
}

func (t *Traefik) cacheGetOrPutEntrypoint(id string) *cacheEntrypoint {
	if _, ok := t.cache.entrypoints[id]; !ok {
		name, proto := id, id
		if idx := strings.LastIndexByte(id, '_'); idx != -1 {
			name, proto = id[:idx], id[idx+1:]
		}
		t.cache.entrypoints[id] = &cacheEntrypoint{
			name:            name,
			proto:           proto,
			reqDurData:      make(map[string]cacheEntrypointReqDur),
			openConnMethods: make(map[string]bool),
		}
	}
	return t.cache.entrypoints[id]
}
