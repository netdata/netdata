// SPDX-License-Identifier: GPL-3.0-or-later

// Package geoip resolves client IPs to ISO country codes plus city-level
// location from an mmdb the Netdata Agent already maintains: the
// topology IP-intel database first, legacy GeoLite2 locations after it.
// Coordinates are rounded before leaving the resolver; client IPs
// are never stored.
package geoip

import (
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/oschwald/maxminddb-golang"
)

// DefaultPaths is the search order when rum.geoip_db is not set:
// the downloader's refreshed copy first, then the stock copy netdata
// packages ship under /usr/share (netflow-plugin's own default paths,
// src/crates/netflow-plugin/src/plugin_config/defaults.rs +
// process_maps.rs, read the same two locations), then legacy GeoLite2
// Country fallbacks (country only: no city or map position).
var DefaultPaths = []string{
	"/var/cache/netdata/topology-ip-intel/topology-ip-geo.mmdb",
	"/usr/share/netdata/topology-ip-intel/topology-ip-geo.mmdb",
	"/var/lib/netdata/geoip/GeoLite2-Country.mmdb",
	"/usr/share/GeoIP/GeoLite2-Country.mmdb",
}

// ErrNotFound means no candidate database exists (GeoIP simply stays off).
var ErrNotFound = errors.New("geoip: no database found")

// Resolve picks the database path: an explicit override must exist (a
// typo must not silently fall back), otherwise the first existing
// candidate wins.
func Resolve(override string, candidates []string) (string, error) {
	if override != "" {
		if _, err := os.Stat(override); err != nil {
			return "", fmt.Errorf("geoip: configured database %s: %w", override, err)
		}
		return override, nil
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", ErrNotFound
}

// Open resolves and opens the database. A nil resolver with ErrNotFound
// means "run without GeoIP".
func Open(override string) (*Resolver, error) {
	p, err := Resolve(override, DefaultPaths)
	if err != nil {
		return nil, err
	}
	return New(p)
}

// Resolver provides IP → country lookups from a GeoLite2-layout mmdb.
type Resolver struct {
	mu sync.RWMutex
	db *maxminddb.Reader
}

// geoRecord decodes the fields this package reads from a GeoLite2/DB-IP
// City-layout mmdb: DB-IP Lite and GeoLite2 City both
// key the English city name and coordinates the same way.
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
		Latitude  float64 `maxminddb:"latitude"`
		Longitude float64 `maxminddb:"longitude"`
	} `maxminddb:"location"`
}

func New(dbPath string) (*Resolver, error) {
	r := &Resolver{}
	if err := r.Load(dbPath); err != nil {
		return nil, err
	}
	return r, nil
}

// Load opens or replaces the database file.
func (r *Resolver) Load(dbPath string) error {
	if dbPath == "" {
		return nil
	}
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("geoip db not found: %s", dbPath)
	}
	db, err := maxminddb.Open(dbPath)
	if err != nil {
		return fmt.Errorf("geoip open: %w", err)
	}
	r.mu.Lock()
	if r.db != nil {
		_ = r.db.Close()
	}
	r.db = db
	r.mu.Unlock()
	return nil
}

// DatabaseType is the mmdb's declared type, e.g. "GeoLite2-City".
func (r *Resolver) DatabaseType() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.db == nil {
		return ""
	}
	return r.db.Metadata.DatabaseType
}

// CountryOnly reports a Country-edition database: countries resolve, but
// cities and map positions stay empty.
func (r *Resolver) CountryOnly() bool { return countryOnlyType(r.DatabaseType()) }

func countryOnlyType(dbType string) bool {
	return strings.Contains(strings.ToLower(dbType), "country")
}

// Country returns the ISO country code for an IP (empty if unknown).
func (r *Resolver) Country(ipStr string) string {
	country, _, _, _, _ := r.Lookup(ipStr)
	return country
}

// Lookup resolves country plus city-level location for an IP in a single
// mmdb read. Coordinates are rounded to 1 decimal
// degree before they leave this package; this reduces precision, not database error. The IP itself is never
// stored or returned. hasGeo is false when the record carries no
// location block — private ranges, some hosting/anycast allocations, or
// no database at all — city/lat/lon are then the zero value.
//
// Coordinates that decode to exactly (0,0) are treated as "no location"
// (hasGeo=false): the maxminddb decoder cannot distinguish a genuinely
// absent location field from one that happens to be the zero value, and
// no real visitor traffic is expected from Null Island.
func (r *Resolver) Lookup(ipStr string) (country, city string, lat, lon float64, hasGeo bool) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "", "", 0, 0, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.db == nil {
		return "", "", 0, 0, false
	}
	var rec geoRecord
	if err := r.db.Lookup(ip, &rec); err != nil {
		return "", "", 0, 0, false
	}
	country = rec.Country.ISOCode
	city = trimCityAnnex(rec.City.Names.En)
	if rec.Location.Latitude != 0 || rec.Location.Longitude != 0 {
		lat = round1(rec.Location.Latitude)
		lon = round1(rec.Location.Longitude)
		hasGeo = true
	}
	return country, city, lat, lon, hasGeo
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

// Close releases the database.
func (r *Resolver) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.db != nil {
		_ = r.db.Close()
		r.db = nil
	}
}
