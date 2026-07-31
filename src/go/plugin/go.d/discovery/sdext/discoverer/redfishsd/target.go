// SPDX-License-Identifier: GPL-3.0-or-later

package redfishsd

import (
	"fmt"
	"sort"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
)

type targetGroup struct {
	provider string
	source   string
	mu       sync.Mutex
	targets  []model.Target
}

func (g *targetGroup) Provider() string        { return g.provider }
func (g *targetGroup) Source() string          { return g.source }
func (g *targetGroup) Targets() []model.Target { return g.targets }

func (g *targetGroup) add(value *target) {
	g.mu.Lock()
	g.targets = append(g.targets, value)
	g.mu.Unlock()
}

func (g *targetGroup) sort() {
	g.mu.Lock()
	sort.Slice(g.targets, func(i, j int) bool { return g.targets[i].TUID() < g.targets[j].TUID() })
	g.mu.Unlock()
}

type target struct {
	model.Base `hash:"ignore"`
	hash       uint64

	IPAddress   string
	Port        int
	Scheme      string
	URL         string
	EndpointKey string
	Profile     string
	JobConfig   map[string]any
}

func newTarget(candidate endpointCandidate) *target {
	value := &target{
		IPAddress: candidate.address.String(),
		Port:      candidate.port, Scheme: candidate.profile.Scheme,
		URL: candidate.url, EndpointKey: candidate.key,
		Profile:   candidate.profile.Name,
		JobConfig: cloneMap(candidate.profile.JobConfig),
	}
	hashMaterial := struct {
		URL         string
		EndpointKey string
		Profile     string
		Revision    uint64
		JobConfig   map[string]any
	}{
		URL: candidate.url, EndpointKey: candidate.key, Profile: candidate.profile.Name,
		Revision:  candidate.profile.hashRevision,
		JobConfig: targetHashConfig(candidate.profile.JobConfig),
	}
	value.hash, _ = model.CalcHash(hashMaterial)
	return value
}

func (t *target) TUID() string { return fmt.Sprintf("redfish_%s", t.EndpointKey) }
func (t *target) Hash() uint64 { return t.hash }

func targetHashConfig(source map[string]any) map[string]any {
	result := cloneMap(source)
	for _, key := range []string{"username", "password", "tls_key"} {
		if _, configured := result[key]; configured {
			result[key] = "configured"
		}
	}
	if proxy, configured := result["proxy_url"]; configured {
		result["proxy_url"] = cacheSafeProxyURL(proxy)
	}
	return result
}
