// SPDX-License-Identifier: GPL-3.0-or-later

// Package geoip consumes optional, locally provisioned geographic databases for
// RUM. The receiver owns refresh and shutdown; lookups never perform file I/O.
// Coordinates lose precision before leaving this package; client IPs are not retained.
package geoip

import (
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"sync/atomic"
	"time"
)

// Paths are injected from the Agent's runtime or compiled data roots.
type Paths struct{ Cache, Stock string }

func AgentPaths(cacheDir, stockDir string) Paths {
	path := func(root string) string {
		if root == "" {
			return ""
		}
		return filepath.Join(root, "topology-ip-intel", "topology-ip-geo.mmdb")
	}
	return Paths{
		Cache: path(cacheDir),
		Stock: path(stockDir),
	}
}

// Status is source evidence, independent of any site's capture policy. Times are
// Unix microseconds, omitted before the corresponding event. No paths or IPs escape.
type Status struct {
	Selection     string `json:"selection,omitempty"`
	State         string `json:"state"`
	Source        string `json:"source,omitempty"`
	DatabaseType  string `json:"database_type,omitempty"`
	BuildAt       int64  `json:"build_at,omitempty"`
	LoadedAt      int64  `json:"loaded_at,omitempty"`
	LastCheckedAt int64  `json:"last_checked_at,omitempty"`
	Reason        string `json:"reason,omitempty"`
	LookupErrors  uint64 `json:"lookup_errors"`
}

type source struct{ kind, path string }
type snapshot struct {
	source   source
	info     os.FileInfo
	db       *database
	loadedAt int64
	tainted  atomic.Bool
	errors   atomic.Uint64
}

// Resolver serializes refresh/close separately from lookups. Candidate loading
// does not block current lookups; publication waits for their mapped reads to end.
type Resolver struct {
	refreshMu sync.Mutex
	mu        sync.RWMutex
	sources   []source
	current   *snapshot
	status    Status
	closed    bool
	wake      chan struct{}
}

func New(explicit string, paths Paths) *Resolver {
	r := &Resolver{
		wake: make(chan struct{}, 1),
		status: Status{
			Selection: "auto",
			State:     "unavailable",
			Reason:    "not_checked",
		},
	}
	if explicit != "" {
		r.status.Selection = "explicit"
		r.sources = []source{{kind: "explicit", path: explicit}}
	} else {
		r.sources = []source{{kind: "cache", path: paths.Cache}, {kind: "stock", path: paths.Stock}}
	}
	return r
}

// Faults wakes the receiver worker after a contained fault. It carries no IP data.
func (r *Resolver) Faults() <-chan struct{} { return r.wake }

// Refresh checks current candidates in priority order, then considers retaining
// a previous readable snapshot. Missing sources and optional load failures never
// become receiver startup failures. Returned errors are for local diagnostics only.
func (r *Resolver) Refresh() error {
	r.refreshMu.Lock()
	defer r.refreshMu.Unlock()
	if r.closed {
		return nil
	}
	r.mu.RLock()
	previous := r.current
	r.mu.RUnlock()
	var selected *snapshot
	var problems []error
	reason := ""
	previousRemoved := false
	for _, src := range r.sources {
		if src.path == "" {
			continue
		}
		info, err := os.Stat(src.path)
		if errors.Is(err, os.ErrNotExist) {
			if previous != nil && previous.source == src {
				previousRemoved = true
			}
			continue
		}
		if err == nil && previous != nil && previous.source == src && sameFile(previous.info, info) &&
			!previous.tainted.Load() {
			selected = previous
			break
		}
		// An in-place change affects the old mapping too; it cannot be last-good data.
		if err == nil && previous != nil && previous.source == src && os.SameFile(previous.info, info) &&
			!sameFile(previous.info, info) {
			previous.tainted.Store(true)
		}
		var db *database
		var opened os.FileInfo
		if err == nil {
			db, opened, err = openDatabase(src.path)
			if err == nil {
				after, statErr := os.Stat(src.path)
				if errors.Is(statErr, os.ErrNotExist) && previous != nil && previous.source == src {
					previousRemoved = true
				}
				if statErr != nil || !sameFile(info, opened) || !sameFile(opened, after) {
					db.close()
					db = nil
					err = errors.New("database changed during opening")
				}
			}
		}
		if err != nil {
			if errors.Is(err, os.ErrNotExist) && previous != nil && previous.source == src {
				previousRemoved = true
			}
			if reason == "" {
				reason = src.kind + "_load_failed"
				if errors.Is(err, errMappedFault) {
					reason = src.kind + "_mapped_fault"
				}
			}
			problems = append(problems, fmt.Errorf("%s database %q: %w", src.kind, src.path, err))
			continue
		}
		selected = &snapshot{
			source:   src,
			info:     opened,
			db:       db,
			loadedAt: time.Now().UnixMicro(),
		}
		break
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state := "loaded"
	if selected != nil && selected.tainted.Load() {
		selected = nil
	}
	if selected == nil && previous != nil && !previousRemoved && !previous.tainted.Load() {
		selected = previous
		state = "using_previous"
	}
	if selected == nil {
		state = "unavailable"
		if reason == "" {
			reason = "missing"
		}
	}
	if previous != nil && previous != selected {
		previous.db.close()
	}
	r.current = selected
	r.status = Status{
		Selection:     r.status.Selection,
		State:         state,
		LastCheckedAt: time.Now().UnixMicro(),
		Reason:        reason,
	}
	return errors.Join(problems...)
}

func sameFile(a, b os.FileInfo) bool {
	return a != nil && b != nil && os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime() == b.ModTime()
}

func (r *Resolver) Status() Status {
	r.mu.RLock()
	defer r.mu.RUnlock()
	status := r.status
	if current := r.current; current != nil {
		status.LookupErrors = current.errors.Load()
		if current.tainted.Load() {
			status.State, status.Reason = "unavailable", "unsafe_snapshot"
			return status
		}
		status.Source = current.source.kind
		status.DatabaseType = current.db.reader.Metadata.DatabaseType
		status.BuildAt = int64(current.db.reader.Metadata.BuildEpoch) * 1000000
		status.LoadedAt = current.loadedAt
	}
	return status
}

type geoRecord struct {
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
	City struct {
		Names struct {
			En string `maxminddb:"en"`
		} `maxminddb:"names"`
	} `maxminddb:"city"`
	Location struct {
		Latitude  *float64 `maxminddb:"latitude"`
		Longitude *float64 `maxminddb:"longitude"`
	} `maxminddb:"location"`
}

// Lookup returns only validated fields. A record decode failure loses this lookup,
// not the entire source; a mapped-read fault retires the unsafe source instead.
func (r *Resolver) Lookup(ipStr string) (country, city string, lat, lon float64, hasGeo bool) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	current := r.current
	if current == nil || current.tainted.Load() || (current.db.reader.Metadata.IPVersion == 4 && ip.To4() == nil) {
		return
	}
	var rec geoRecord
	if err := current.db.lookup(ip, &rec); err != nil {
		current.errors.Add(1)
		if errors.Is(err, errMappedFault) {
			current.tainted.Store(true)
			select {
			case r.wake <- struct{}{}:
			default:
			}
		}
		return
	}
	invalid := false
	country = rec.Country.ISOCode
	if country != "" &&
		(len(country) != 2 || country[0] < 'A' || country[0] > 'Z' || country[1] < 'A' || country[1] > 'Z') {
		country = ""
		invalid = true
	}
	city = trimCityAnnex(rec.City.Names.En)
	if rec.Location.Latitude != nil && rec.Location.Longitude != nil {
		a, b := *rec.Location.Latitude, *rec.Location.Longitude
		if !math.IsNaN(a) && !math.IsInf(a, 0) && a >= -90 && a <= 90 && !math.IsNaN(b) && !math.IsInf(b, 0) &&
			b >= -180 &&
			b <= 180 {
			lat, lon, hasGeo = round1(a), round1(b), true
		} else {
			invalid = true
		}
	}
	if invalid {
		current.errors.Add(1)
	}
	return
}

// round1 rounds to 1 decimal degree before coordinates leave the resolver.
func round1(f float64) float64 {
	r := math.Round(f*10) / 10
	if r == 0 {
		return 0 // normalise -0, which JSON would emit literally
	}
	return r
}

// cityAnnexRe matches a trailing " (...)" qualifier DB-IP appends to some
// city.names.en values (neighbourhood/district/ward disambiguation), e.g.
// "Lagos (Victoria Island Annex)", "Naju (Jinheung-gil)",
// "Fujisawa (Endō)". Requires at least one non-space character before the
// " (" so a name that is only a parenthetical (no base name) is left
// alone, and the group must run to the end of the string so an
// unrelated/unclosed "(" elsewhere in the name is not touched.
var cityAnnexRe = regexp.MustCompile(`^(.*\S) \([^()]*\)$`)

// trimCityAnnex strips a trailing " (...)" qualifier, keeping the base
// city name; names without one are returned unchanged.
func trimCityAnnex(name string) string {
	if m := cityAnnexRe.FindStringSubmatch(name); m != nil {
		return m[1]
	}
	return name
}

// Close joins refresh and mapped reads before releasing the current database.
func (r *Resolver) Close() {
	r.refreshMu.Lock()
	defer r.refreshMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	if r.current != nil {
		r.current.db.close()
		r.current = nil
	}
	r.status.State, r.status.Reason = "unavailable", "closed"
}
