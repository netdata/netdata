// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
)

func Test_Collector_resolveApp(t *testing.T) {
	const hap, ngx = "haproxy", "nginx"

	tests := map[string]struct {
		configApp string
		jobName   string
		profiles  []promprofiles.Profile
		want      string
		wantWarn  bool
	}{
		"config app wins over a profile app": {
			configApp: "myapp", jobName: "job",
			profiles: []promprofiles.Profile{{Name: hap, App: hap}},
			want:     "myapp",
		},
		"config app silences a profile-app conflict": {
			configApp: "myapp", jobName: "job",
			profiles: []promprofiles.Profile{{Name: hap, App: hap}, {Name: ngx, App: ngx}},
			want:     "myapp",
		},
		"profile app when no config app": {
			jobName:  "job",
			profiles: []promprofiles.Profile{{Name: hap, App: hap}},
			want:     hap,
		},
		"first profile app wins on conflict": {
			jobName:  "job",
			profiles: []promprofiles.Profile{{Name: hap, App: hap}, {Name: ngx, App: ngx}},
			want:     hap,
			wantWarn: true,
		},
		"shared profiles (no app) fall through to job name": {
			jobName:  "job",
			profiles: []promprofiles.Profile{{Name: "go_runtime"}, {Name: "http"}},
			want:     "job",
		},
		"a profile app is used even after shared profiles": {
			jobName:  "job",
			profiles: []promprofiles.Profile{{Name: "go_runtime"}, {Name: hap, App: hap}},
			want:     hap,
		},
		"job name when no config app and no profile declares one": {
			jobName:  "job",
			profiles: nil,
			want:     "job",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var logBuf bytes.Buffer
			c := New()
			c.Logger = logger.NewWithWriter(&logBuf)
			c.Application = tc.configApp
			c.Name = tc.jobName

			assert.Equal(t, tc.want, c.resolveApp(tc.profiles))

			// Conflicting profile apps must emit exactly one operator warning; every other
			// case (including config-app-wins, which short-circuits before the loop) stays silent.
			if tc.wantWarn {
				assert.Contains(t, logBuf.String(), "different apps")
			} else {
				assert.NotContains(t, logBuf.String(), "different apps")
			}
		})
	}
}

func Test_profileMatchesFamilies(t *testing.T) {
	mfs := testMetricFamilies("haproxy_up", "haproxy_frontend_status")

	tests := map[string]struct {
		match string
		want  bool
	}{
		"match hits a family":       {match: "haproxy_*", want: true},
		"match misses every family": {match: "nginx_*", want: false},
		"exact-name match":          {match: "haproxy_up", want: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ok, err := profileMatchesFamilies(promprofiles.Profile{Name: "p", Match: tc.match}, mfs)
			require.NoError(t, err)
			assert.Equal(t, tc.want, ok)
		})
	}
}

func Test_autoSelectProfiles(t *testing.T) {
	profiles := []promprofiles.Profile{
		{Name: "haproxy", Match: "haproxy_*"},
		{Name: "nginx", Match: "nginx_*"},
	}

	tests := map[string]struct {
		mfs  prometheus.MetricFamilies
		want []string
	}{
		"keeps only profiles whose match hits a scraped family": {
			mfs:  testMetricFamilies("haproxy_up"),
			want: []string{"haproxy"},
		},
		"keeps every matching profile in catalog order": {
			mfs:  testMetricFamilies("haproxy_up", "nginx_connections"),
			want: []string{"haproxy", "nginx"},
		},
		"selects nothing when no family matches": {
			mfs:  testMetricFamilies("redis_up"),
			want: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := autoSelectProfiles(profiles, tc.mfs)
			require.NoError(t, err)
			assert.Equal(t, tc.want, profileNames(got))
		})
	}
}

func Test_namedSelectProfiles(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"haproxy": testProfileYAML("haproxy_*"),
		"nginx":   testProfileYAML("nginx_*"),
	})
	mfs := testMetricFamilies("haproxy_up")

	tests := map[string]struct {
		names   []string
		want    []string
		wantErr bool
	}{
		"resolves and returns the named profile": {
			names: []string{"haproxy"},
			want:  []string{"haproxy"},
		},
		"errors when a named profile matches nothing": {
			names:   []string{"nginx"},
			wantErr: true,
		},
		"errors on an unknown name": {
			names:   []string{"does_not_exist"},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := namedSelectProfiles(catalog, tc.names, mfs)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, profileNames(got))
		})
	}
}

func Test_combinedSelectProfiles(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"haproxy": testProfileYAML("haproxy_*"),
		"nginx":   testProfileYAML("nginx_*"),
	})
	mfs := testMetricFamilies("haproxy_up")

	tests := map[string]struct {
		names   []string
		want    []string
		wantErr bool
	}{
		"dedups a profile selected by both auto and name": {
			names: []string{"haproxy"},
			want:  []string{"haproxy"},
		},
		"dedups case-insensitively": {
			names: []string{"HAPROXY"},
			want:  []string{"haproxy"},
		},
		"propagates a named no-match error": {
			names:   []string{"nginx"},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := combinedSelectProfiles(catalog, tc.names, mfs)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, profileNames(got))
		})
	}
}

// testMetricFamilies builds a MetricFamilies whose only meaningful content is the
// set of family names (selection matches on the keys).
func testMetricFamilies(names ...string) prometheus.MetricFamilies {
	mfs := make(prometheus.MetricFamilies, len(names))
	for _, n := range names {
		mfs[n] = nil
	}
	return mfs
}

func profileNames(profiles []promprofiles.Profile) []string {
	if len(profiles) == 0 {
		return nil
	}
	names := make([]string, 0, len(profiles))
	for _, p := range profiles {
		names = append(names, p.Name)
	}
	return names
}

// loadTestCatalog writes the given <name>.yaml profiles to a temp stock dir and
// loads them, so named/combined selection can be tested without the on-disk
// stock catalog.
func loadTestCatalog(t *testing.T, profiles map[string]string) promprofiles.Catalog {
	t.Helper()

	dir := t.TempDir()
	for name, data := range profiles {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(data), 0o600))
	}

	catalog, err := promprofiles.LoadFromDirs([]promprofiles.DirSpec{{Path: dir, IsStock: true}})
	require.NoError(t, err)
	return catalog
}

// testProfileYAML is a minimal valid profile body with the given match pattern.
func testProfileYAML(match string) string {
	return strings.Join([]string{
		"match: '" + match + "'",
		"template:",
		"  family: Test",
		"  context_namespace: test",
		"  metrics:",
		"    - test_up",
		"  charts:",
		"    - title: Up",
		"      context: up",
		"      units: status",
		"      algorithm: absolute",
		"      dimensions:",
		"        - selector: test_up",
		"          name: up",
	}, "\n") + "\n"
}
