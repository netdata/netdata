// SPDX-License-Identifier: GPL-3.0-or-later
package aggregate

import (
	"net"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"golang.org/x/net/publicsuffix"
)

const resourceHostTopN = 10
const apiName = "__api"

type hostKey struct {
	host    string
	unknown bool
}
type resourcePopulation struct {
	count  uint64
	values []float64
}
type ResourceHostGroup struct {
	Host     string
	Other    bool
	Unknown  bool // Missing host metadata; distinct from a literal host named "unknown".
	Count    uint64
	Duration VitalStats
	Lost     uint64
}

func isAPICall(pageHost string, r beacon.Resource) bool {
	return !r.Self && r.HasDuration && validMeasurement(r.DurationMS) &&
		(r.Initiator == "fetch" || r.Initiator == "xmlhttprequest") &&
		isFirstParty(pageHost, r.Host)
}

// isFirstParty reports whether resHost is the same registrable domain as
// pageHost. publicsuffix is already a transitive
// dependency (golang.org/x/net, via grpc) so no new module is added.
// Hosts publicsuffix cannot classify (bare IPs, "localhost") fall back to
// an exact host match, which is the strictest possible definition of
// "same site" for those inputs. Faro's httpHost carries a non-default
// port ("www.example.com:8443") while pageHost never does, so ports are
// ignored: the registrable domain decides, as for cookies.
func isFirstParty(pageHost, resHost string) bool {
	if pageHost == "" || resHost == "" {
		return false
	}
	pageHost, resHost = strings.ToLower(stripPort(pageHost)), strings.ToLower(stripPort(resHost))
	if pageHost == resHost {
		return true
	}
	pe, err1 := publicsuffix.EffectiveTLDPlusOne(pageHost)
	re, err2 := publicsuffix.EffectiveTLDPlusOne(resHost)
	if err1 != nil || err2 != nil {
		return false
	}
	return pe == re
}

// stripPort drops a ":port" suffix, leaving bare and bracketed IPv6 hosts
// ("::1", "[::1]") as they are unless a port follows the bracket.
func stripPort(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func (r *windowRead) addResource(v *activityObservation) {
	resource := v.resource
	host := resource.Host
	if host == "" {
		host = unknownValue
	}
	key := hostKey{
		host:    host,
		unknown: resource.Host == "",
	}
	g := r.resourceGroups[key]
	if g == nil {
		g = &resourcePopulation{}
		r.resourceGroups[key] = g
	}
	g.count++
	if resource.HasDuration {
		g.values = append(g.values, resource.DurationMS)
	}
	if isAPICall(v.pageHost, *resource) {
		r.global.values[apiName] = append(r.global.values[apiName], resource.DurationMS)
	}
}
func (r *windowRead) rankResourceHosts() []ResourceHostGroup {
	var keys []hostKey
	for key := range r.resourceGroups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := r.resourceGroups[keys[i]], r.resourceGroups[keys[j]]
		if a.count != b.count {
			return a.count > b.count
		}
		if keys[i].host != keys[j].host {
			return keys[i].host < keys[j].host
		}
		return !keys[i].unknown && keys[j].unknown
	})
	n := min(resourceHostTopN, len(keys))
	var out []ResourceHostGroup
	for _, key := range keys[:n] {
		g := r.resourceGroups[key]
		out = append(out, ResourceHostGroup{
			Host:     key.host,
			Unknown:  key.unknown,
			Count:    g.count,
			Duration: measurementStats(apiName, g.values, r.resourcesLost),
			Lost:     r.resourcesLost,
		})
	}
	other := resourcePopulation{}
	present := false
	for _, key := range keys[n:] {
		g := r.resourceGroups[key]
		other.count += g.count
		other.values = append(other.values, g.values...)
		present = true
	}
	if present {
		out = append(out, ResourceHostGroup{
			Host:     Other,
			Other:    true,
			Count:    other.count,
			Duration: measurementStats(apiName, other.values, r.resourcesLost),
			Lost:     r.resourcesLost,
		})
	}
	return out
}
