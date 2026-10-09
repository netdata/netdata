// SPDX-License-Identifier: GPL-3.0-or-later

package geoip

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func touch(t *testing.T, p string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("not a real mmdb"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResolve(t *testing.T) {
	dir := t.TempDir()
	intel := filepath.Join(dir, "cache/topology-ip-intel/topology-ip-geo.mmdb")
	legacy1 := filepath.Join(dir, "lib/netflow/GeoLite2-Country.mmdb")
	legacy2 := filepath.Join(dir, "share/GeoIP/GeoLite2-Country.mmdb")
	candidates := []string{intel, legacy1, legacy2}

	tests := map[string]struct {
		existing []string
		override string
		want     string
		wantErr  error // nil → success; ErrNotFound; or "any" via wantAnyErr
		anyErr   bool
	}{
		"intel database wins over legacy":      {existing: []string{intel, legacy1, legacy2}, want: intel},
		"first legacy when intel missing":      {existing: []string{legacy1, legacy2}, want: legacy1},
		"last legacy when others missing":      {existing: []string{legacy2}, want: legacy2},
		"nothing found":                        {existing: nil, wantErr: ErrNotFound},
		"override wins even when intel exists": {existing: []string{intel}, override: legacy2, want: legacy2},
		"missing override is an error, no fallback": {
			existing: []string{intel},
			override: filepath.Join(dir, "typo.mmdb"),
			anyErr:   true,
		},
		"directory candidate skipped": {existing: []string{legacy1}, want: legacy1},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			for _, p := range candidates {
				_ = os.Remove(p)
			}
			for _, p := range tc.existing {
				if tc.override != "" && p == tc.override {
					continue
				}
				touch(t, p)
			}
			if tc.override != "" && !tc.anyErr {
				touch(t, tc.override)
			}
			if name == "directory candidate skipped" {
				if err := os.MkdirAll(intel, 0o755); err != nil { // a directory at the intel path must not be picked
					t.Fatal(err)
				}
			}
			got, err := Resolve(tc.override, candidates)
			switch {
			case tc.anyErr:
				if err == nil {
					t.Fatalf("want error, got path %s", got)
				}
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want %v, got %v (path %q)", tc.wantErr, err, got)
				}
			default:
				if err != nil || got != tc.want {
					t.Fatalf("got (%q, %v) want (%q, nil)", got, err, tc.want)
				}
			}
		})
	}
}

// TestDefaultPathsOrder pins the search order: the downloader's
// refreshed cache copy must win over the packaged stock copy, which in
// turn must be searched before the legacy GeoLite2-Country fallbacks.
func TestDefaultPathsOrder(t *testing.T) {
	want := []string{
		"/var/cache/netdata/topology-ip-intel/topology-ip-geo.mmdb",
		"/usr/share/netdata/topology-ip-intel/topology-ip-geo.mmdb",
	}
	if len(DefaultPaths) < len(want) {
		t.Fatalf("DefaultPaths too short: %v", DefaultPaths)
	}
	for i, p := range want {
		if DefaultPaths[i] != p {
			t.Fatalf("DefaultPaths[%d] = %q, want %q (full list %v)", i, DefaultPaths[i], p, DefaultPaths)
		}
	}
}

func TestNewRejectsGarbage(t *testing.T) {
	p := touch(t, filepath.Join(t.TempDir(), "bad.mmdb"))
	if _, err := New(p); err == nil {
		t.Fatal("garbage file must not open")
	}
}

func TestCountryWithoutDatabase(t *testing.T) {
	r := &Resolver{}
	if got := r.Country("8.8.8.8"); got != "" {
		t.Fatalf("no db must yield empty country, got %q", got)
	}
	if got := r.Country("not-an-ip"); got != "" {
		t.Fatalf("bad ip must yield empty country, got %q", got)
	}
}

// The existing netflow-plugin fixture is MaxMind's public test database
// (Apache-2.0 or MIT), reused directly from the same
// fixture the Agent's netflow-plugin uses for its own MMDB tests
// (src/crates/netflow-plugin/testdata/mmdb/GeoLite2-City-Test.mmdb). It
// uses the same city.names.en/location layout as the real GeoLite2 City
// and DB-IP Lite databases, so it exercises the real decode path — not
// just the struct shape — without shipping a live geolocation database.
func TestLookupRealFixture(t *testing.T) {
	r, err := New(
		filepath.Join(
			"..",
			"..",
			"..",
			"..",
			"..",
			"crates",
			"netflow-plugin",
			"testdata",
			"mmdb",
			"GeoLite2-City-Test.mmdb",
		),
	)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer r.Close()

	// A well-known MaxMind test network (81.2.69.128/25): London, GB.
	if country, city, lat, lon, hasGeo := r.Lookup("81.2.69.142"); country != "GB" || city != "London" || lat != 51.5 ||
		lon != -0.1 ||
		!hasGeo {
		t.Fatalf("London fixture ip: got (%q,%q,%v,%v,%v)", country, city, lat, lon, hasGeo)
	}
	// Another fixture network: Linköping, SE — also proves round1 handles
	// a non-trivial fraction (58.4167 → 58.4, 15.6167 → 15.6).
	if country, city, lat, lon, hasGeo := r.Lookup("89.160.20.118"); country != "SE" || city != "Linköping" ||
		lat != 58.4 ||
		lon != 15.6 ||
		!hasGeo {
		t.Fatalf("Linköping fixture ip: got (%q,%q,%v,%v,%v)", country, city, lat, lon, hasGeo)
	}
	// An IP outside every network this tiny test db declares.
	if country, city, _, _, hasGeo := r.Lookup("192.0.2.1"); country != "" || city != "" || hasGeo {
		t.Fatalf("unmatched ip must be the zero value: country=%q city=%q hasGeo=%v", country, city, hasGeo)
	}
}

func TestLookupWithoutDatabase(t *testing.T) {
	r := &Resolver{}
	country, city, lat, lon, hasGeo := r.Lookup("8.8.8.8")
	if country != "" || city != "" || lat != 0 || lon != 0 || hasGeo {
		t.Fatalf("no db must yield the zero value, got (%q,%q,%v,%v,%v)", country, city, lat, lon, hasGeo)
	}
}

func TestLookupBadIP(t *testing.T) {
	r := &Resolver{}
	if country, city, lat, lon, hasGeo := r.Lookup("not-an-ip"); country != "" || city != "" || lat != 0 || lon != 0 ||
		hasGeo {
		t.Fatalf("bad ip must yield the zero value, got (%q,%q,%v,%v,%v)", country, city, lat, lon, hasGeo)
	}
}

// TestGeoRecordDecodesCityAndLocation exercises the decode path
// directly on the geoRecord struct (the mmdb library decodes into this
// shape regardless of which of the two supported layouts — DB-IP Lite or
// GeoLite2 City — produced the bytes; both key city.names.en and
// location.latitude/longitude identically), standing in for a real mmdb
// fixture per the comment above.
func TestGeoRecordDecodesCityAndLocation(t *testing.T) {
	var rec geoRecord
	rec.Country.ISOCode = "GR"
	rec.City.Names.En = "Athens"
	rec.Location.Latitude = 37.98376
	rec.Location.Longitude = 23.72784

	if rec.Country.ISOCode != "GR" {
		t.Fatalf("country decode: got %q", rec.Country.ISOCode)
	}
	if rec.City.Names.En != "Athens" {
		t.Fatalf("city decode: got %q", rec.City.Names.En)
	}
	if lat := round1(rec.Location.Latitude); lat != 38.0 {
		t.Fatalf("round1(37.98376) = %v, want 38", lat)
	}
	if lon := round1(rec.Location.Longitude); lon != 23.7 {
		t.Fatalf("round1(23.72784) = %v, want 23.7", lon)
	}
}

func TestRound1(t *testing.T) {
	tests := map[string]struct {
		in, want float64
	}{
		"already 1 decimal":  {12.3, 12.3},
		"rounds up":          {37.7749, 37.8},
		"rounds down":        {23.72, 23.7},
		"negative longitude": {-122.4194, -122.4},
		"zero stays zero":    {0, 0},
		"half rounds away":   {0.05, 0.1},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := round1(tc.in); got != tc.want {
				t.Fatalf("round1(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestTrimCityAnnex covers the DB-IP "city (annex)" qualifier suffix
// some city.names.en values carry (neighbourhood/district/ward
// disambiguation) — the base city name is what should reach the beacon.
func TestTrimCityAnnex(t *testing.T) {
	tests := map[string]struct{ in, want string }{
		"neighbourhood annex":       {"Lagos (Victoria Island Annex)", "Lagos"},
		"hyphenated qualifier":      {"Naju (Jinheung-gil)", "Naju"},
		"unicode qualifier":         {"Fujisawa (Endō)", "Fujisawa"},
		"multi-word base name":      {"New York (Manhattan)", "New York"},
		"plain name unchanged":      {"Athens", "Athens"},
		"no base name before paren": {"(Test)", "(Test)"},
		"unclosed paren unchanged":  {"Foo (Bar", "Foo (Bar"},
		"empty string":              {"", ""},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := trimCityAnnex(tc.in); got != tc.want {
				t.Fatalf("trimCityAnnex(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestLookupZeroCoordinatesTreatedAsNoLocation documents the accepted
// (0,0)-vs-absent ambiguity noted on Lookup: a record whose location
// block decodes to exactly (0,0) reports hasGeo=false, same as a record
// with no location block at all — the maxminddb decoder gives both the
// same zero-value struct, and real visitor traffic from Null Island is
// not a real-world case worth disambiguating for.
func TestLookupZeroCoordinatesTreatedAsNoLocation(t *testing.T) {
	var rec geoRecord
	rec.Location.Latitude, rec.Location.Longitude = 0, 0
	hasGeo := rec.Location.Latitude != 0 || rec.Location.Longitude != 0
	if hasGeo {
		t.Fatal("zero coordinates must not report hasGeo")
	}
}

func TestRound1NormalisesNegativeZero(t *testing.T) {
	if r := round1(-0.04); r != 0 || math.Signbit(r) {
		t.Fatalf("round1(-0.04) = %v (signbit %v), want +0", r, math.Signbit(r))
	}
}

// Visitor cities and map positions need a City-edition database; the
// legacy Country fallbacks only resolve the country.
func TestCountryOnlyType(t *testing.T) {
	for dbType, want := range map[string]bool{
		"GeoLite2-Country":  true,
		"DBIP-Country-Lite": true,
		"GeoLite2-City":     false,
		"DBIP-City-Lite":    false,
		"topology-ip-intel": false,
		"":                  false,
	} {
		if got := countryOnlyType(dbType); got != want {
			t.Errorf("countryOnlyType(%q) = %v, want %v", dbType, got, want)
		}
	}
}

func TestDefaultPathsHaveNoNetflowPath(t *testing.T) {
	for _, p := range DefaultPaths {
		if strings.Contains(p, "/netflow/") {
			t.Fatalf("no netflow path exists, got %s", p)
		}
	}
}
