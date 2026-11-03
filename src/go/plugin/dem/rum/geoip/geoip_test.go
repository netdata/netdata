// SPDX-License-Identifier: GPL-3.0-or-later

package geoip

import (
	"math"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fixture = "../../../../../crates/netflow-plugin/testdata/mmdb/GeoLite2-City-Test.mmdb"

func countryRecord(country string) mmdbtype.Map {
	return mmdbtype.Map{
		"country": mmdbtype.Map{
			"iso_code": mmdbtype.String(country),
		},
	}
}

func writeDatabase(t *testing.T, path, edition string, rec mmdbtype.Map) {
	t.Helper()
	tree, err := mmdbwriter.New(
		mmdbwriter.Options{
			DatabaseType:            edition,
			BuildEpoch:              1700000000,
			IPVersion:               4,
			IncludeReservedNetworks: true,
		},
	)
	require.NoError(t, err)
	_, network, err := net.ParseCIDR("192.0.2.0/24")
	require.NoError(t, err)
	require.NoError(t, tree.Insert(network, rec))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	f, err := os.CreateTemp(filepath.Dir(path), "replacement-")
	require.NoError(t, err)
	_, err = tree.WriteTo(f)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.NoError(t, os.Rename(f.Name(), path))
}

func replaceBytes(t *testing.T, path string, data []byte) {
	t.Helper()
	f, err := os.CreateTemp(filepath.Dir(path), "replacement-")
	require.NoError(t, err)
	_, err = f.Write(data)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.NoError(t, os.Rename(f.Name(), path))
}

func country(t *testing.T, r *Resolver, want string) {
	t.Helper()
	got, _, _, _, _ := r.Lookup("192.0.2.1")
	assert.Equal(t, want, got)
}

func TestSourceTransitions(t *testing.T) {
	paths := AgentPaths(filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "stock"))
	r := New("", paths)
	t.Cleanup(r.Close)
	assert.Equal(t, "not_checked", r.Status().Reason)
	require.NoError(t, r.Refresh())
	assert.Equal(t, "missing", r.Status().Reason)
	writeDatabase(t, paths.Stock, "GeoLite2-Country", countryRecord("US"))
	require.NoError(t, os.MkdirAll(filepath.Dir(paths.Cache), 0700))
	replaceBytes(t, paths.Cache, []byte("bad cache"))
	require.Error(t, r.Refresh())
	country(t, r, "US")
	assert.Equal(t, "stock", r.Status().Source)
	assert.Equal(t, "cache_load_failed", r.Status().Reason)
	writeDatabase(t, paths.Cache, "Netdata-Topology-GEO", countryRecord("GB"))
	require.NoError(t, r.Refresh())
	country(t, r, "GB")
	accepted := r.Status()
	assert.Equal(t, "cache", accepted.Source)
	require.NoError(t, r.Refresh())
	assert.Equal(t, accepted.LoadedAt, r.Status().LoadedAt)
	assert.Equal(t, accepted.BuildAt, r.Status().BuildAt)
	assert.GreaterOrEqual(t, r.Status().LastCheckedAt, accepted.LastCheckedAt)
	// Failed cache cannot hide a currently valid stock source, even after cache was loaded.
	replaceBytes(t, paths.Cache, []byte("bad replacement"))
	require.Error(t, r.Refresh())
	country(t, r, "US")
	assert.Equal(t, "loaded", r.Status().State)
	previous := r.Status()
	replaceBytes(t, paths.Stock, []byte("bad stock replacement"))
	require.Error(t, r.Refresh())
	country(t, r, "US")
	assert.Equal(t, "using_previous", r.Status().State)
	assert.Equal(t, previous.LoadedAt, r.Status().LoadedAt)
	require.NoError(t, os.Remove(paths.Stock))
	require.Error(t, r.Refresh())
	country(t, r, "")
	assert.Equal(t, "unavailable", r.Status().State)
	assert.Empty(t, r.Status().Source)
	writeDatabase(t, paths.Stock, "DBIP-Country-Lite", countryRecord("FR"))
	require.Error(t, r.Refresh()) // Cache is still rejected.
	country(t, r, "FR")
	r.Close()
	require.NoError(t, r.Refresh())
	country(t, r, "")
	assert.Equal(t, "closed", r.Status().Reason)
}

func TestExplicitSelectionAndRemoval(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{
		Stock: filepath.Join(dir, "stock.mmdb"),
	}
	writeDatabase(t, paths.Stock, "GeoIP2-Country", countryRecord("US"))
	path := filepath.Join(dir, "explicit.mmdb")
	r := New(path, paths)
	t.Cleanup(r.Close)
	require.NoError(t, r.Refresh())
	country(t, r, "")
	assert.Equal(t, "explicit", r.Status().Selection)
	writeDatabase(t, path, "GeoIP2-Country", countryRecord("GB"))
	require.NoError(t, r.Refresh())
	country(t, r, "GB")
	accepted := r.Status()
	replaceBytes(t, path, []byte("bad replacement"))
	require.Error(t, r.Refresh())
	country(t, r, "GB")
	assert.Equal(t, "using_previous", r.Status().State)
	assert.Equal(t, accepted.LoadedAt, r.Status().LoadedAt)
	require.NoError(t, os.Remove(path))
	require.NoError(t, r.Refresh())
	country(t, r, "")
	assert.Equal(t, "unavailable", r.Status().State)
}

func TestDatabaseTypes(t *testing.T) {
	for _, edition := range []string{"Netdata-Topology-GEO", "GeoLite2-City", "GeoLite2-Country", "GeoIP2-City", "GeoIP2-Country", "DBIP-City-Lite", "DBIP-Country-Lite", "GeoLite2-ASN", "Custom-City"} {
		t.Run(edition, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "db.mmdb")
			writeDatabase(t, p, edition, countryRecord("GB"))
			r := New(p, Paths{})
			t.Cleanup(r.Close)
			err := r.Refresh()
			if edition == "GeoLite2-ASN" || edition == "Custom-City" {
				require.ErrorIs(t, err, errUnsupportedDatabase)
				assert.Equal(t, "unavailable", r.Status().State)
			} else {
				require.NoError(t, err)
				country(t, r, "GB")
				assert.Equal(t, edition, r.Status().DatabaseType)
			}
		})
	}
}

func TestLookupRealFixture(t *testing.T) {
	r := New(fixture, Paths{})
	t.Cleanup(r.Close)
	require.NoError(t, r.Refresh())
	c, city, lat, lon, ok := r.Lookup("81.2.69.142")
	assert.Equal(t, "GB", c)
	assert.Equal(t, "London", city)
	assert.Equal(t, 51.5, lat)
	assert.Equal(t, -0.1, lon)
	assert.True(t, ok)
	c, city, lat, lon, ok = r.Lookup("89.160.20.118")
	assert.Equal(t, "SE", c)
	assert.Equal(t, "Linköping", city)
	assert.Equal(t, 58.4, lat)
	assert.Equal(t, 15.6, lon)
	assert.True(t, ok)
	for _, ip := range []string{"192.0.2.1", "not-an-ip"} {
		c, city, lat, lon, ok = r.Lookup(ip)
		assert.Empty(t, c)
		assert.Empty(t, city)
		assert.Zero(t, lat)
		assert.Zero(t, lon)
		assert.False(t, ok)
	}
	assert.Zero(t, r.Status().LookupErrors)
}

func TestLookupFieldValidation(t *testing.T) {
	for _, tc := range []struct {
		name          string
		record        mmdbtype.Map
		country, city string
		location      bool
		errors        uint64
	}{
		{name: "country only", record: countryRecord("GB"), country: "GB"},
		{name: "zero coordinates", record: mmdbtype.Map{
			"location": mmdbtype.Map{
				"latitude":  mmdbtype.Float64(0),
				"longitude": mmdbtype.Float64(-0.01),
			},
		}, location: true},
		{name: "missing longitude", record: mmdbtype.Map{
			"location": mmdbtype.Map{
				"latitude": mmdbtype.Float64(10),
			},
		}},
		{name: "invalid coordinate keeps country", record: mmdbtype.Map{
			"country": mmdbtype.Map{
				"iso_code": mmdbtype.String("GB"),
			},
			"location": mmdbtype.Map{
				"latitude":  mmdbtype.Float64(100),
				"longitude": mmdbtype.Float64(0),
			},
		}, country: "GB", errors: 1},
		{name: "nan coordinate", record: mmdbtype.Map{
			"location": mmdbtype.Map{
				"latitude":  mmdbtype.Float64(math.NaN()),
				"longitude": mmdbtype.Float64(0),
			},
		}, errors: 1},
		{name: "invalid country keeps city", record: mmdbtype.Map{
			"country": mmdbtype.Map{
				"iso_code": mmdbtype.String("bogus"),
			},
			"city": mmdbtype.Map{
				"names": mmdbtype.Map{
					"en": mmdbtype.String("London (district)"),
				},
			},
		}, city: "London", errors: 1},
		{name: "decode error", record: mmdbtype.Map{
			"country": mmdbtype.Map{
				"iso_code": mmdbtype.Uint32(42),
			},
		}, errors: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "db.mmdb")
			writeDatabase(t, p, "Netdata-Topology-GEO", tc.record)
			r := New(p, Paths{})
			t.Cleanup(r.Close)
			require.NoError(t, r.Refresh()) // Loading does not claim full-record validation.
			c, city, lat, lon, ok := r.Lookup("192.0.2.1")
			assert.Equal(t, tc.country, c)
			assert.Equal(t, tc.city, city)
			assert.Equal(t, tc.location, ok)
			assert.Zero(t, lat)
			assert.Zero(t, lon)
			assert.False(t, math.Signbit(lon))
			assert.Equal(t, tc.errors, r.Status().LookupErrors)
			assert.Equal(t, "loaded", r.Status().State)
			require.NoError(t, r.Refresh())
			assert.Equal(t, tc.errors, r.Status().LookupErrors)
			c, _, _, _, ok = r.Lookup("2001:db8::1") // IPv6 against IPv4 database is absence.
			assert.Empty(t, c)
			assert.False(t, ok)
			assert.Equal(t, tc.errors, r.Status().LookupErrors)
			writeDatabase(t, p, "Netdata-Topology-GEO", countryRecord("US"))
			require.NoError(t, r.Refresh())
			assert.Zero(t, r.Status().LookupErrors)
		})
	}
}

func TestConcurrentReplacementAndClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db.mmdb")
	writeDatabase(t, path, "Netdata-Topology-GEO", countryRecord("GB"))
	r := New(path, Paths{})
	t.Cleanup(r.Close)
	require.NoError(t, r.Refresh())
	stop := make(chan struct{})
	var readers sync.WaitGroup
	for range 8 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				c, _, _, _, _ := r.Lookup("192.0.2.1")
				if c != "" && c != "GB" && c != "US" {
					t.Errorf("invalid country %q", c)
					return
				}
				_ = r.Status()
			}
		}()
	}
	for i := range 20 {
		code := "GB"
		if i%2 == 0 {
			code = "US"
		}
		writeDatabase(t, path, "Netdata-Topology-GEO", countryRecord(code))
		require.NoError(t, r.Refresh())
		country(t, r, code)
	}
	r.Close()
	close(stop)
	readers.Wait()
	country(t, r, "")
}

func BenchmarkLookup(b *testing.B) {
	path := os.Getenv("DEM_GEOIP_BENCH_DB")
	if path == "" {
		path = fixture
	}
	r := New(path, Paths{})
	b.Cleanup(r.Close)
	if err := r.Refresh(); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		r.Lookup("81.2.69.142")
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

func TestRound1NormalisesNegativeZero(t *testing.T) {
	if r := round1(-0.04); r != 0 || math.Signbit(r) {
		t.Fatalf("round1(-0.04) = %v (signbit %v), want +0", r, math.Signbit(r))
	}
}
