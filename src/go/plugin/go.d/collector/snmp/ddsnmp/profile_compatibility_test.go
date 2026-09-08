// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadProfile_LegacyScalarInheritance(t *testing.T) {
	for _, section := range []string{"metrics", "topology"} {
		t.Run(section, func(t *testing.T) {
			row := func(legacy bool, description string) string {
				kind := ""
				if section == "topology" {
					kind = "    kind: if_name\n"
				}
				if legacy {
					return fmt.Sprintf("%s:\n  - OID: 1.2.3.0\n    name: example\n%s", section, kind)
				}
				return fmt.Sprintf(
					"%s:\n  - symbol: {OID: 1.2.3.0, name: example}\n%s    MIB: %s\n",
					section,
					kind,
					description,
				)
			}
			for name, tc := range map[string]struct{ base, middle, child, mib string }{
				"legacy base":                        {base: row(true, ""), child: "extends: [base.yaml]\n"},
				"legacy grandparent":                 {base: row(true, ""), middle: "extends: [base.yaml]\n", child: "extends: [middle.yaml]\n"},
				"modern child overrides legacy":      {base: row(true, ""), child: "extends: [base.yaml]\n" + row(false, "child"), mib: "child"},
				"legacy child overrides modern":      {base: row(false, "base"), child: "extends: [base.yaml]\n" + row(true, "")},
				"later legacy base overrides modern": {base: row(false, "base"), middle: row(true, ""), child: "extends: [base.yaml, middle.yaml]\n"},
			} {
				t.Run(name, func(t *testing.T) {
					dir := t.TempDir()
					for file, raw := range map[string]string{"base.yaml": tc.base, "middle.yaml": tc.middle, "child.yaml": tc.child} {
						require.NoError(t, os.WriteFile(filepath.Join(dir, file), []byte(raw), 0600))
					}
					prof, err := loadProfile(filepath.Join(dir, "child.yaml"), multipath.New(dir))
					require.NoError(t, err)
					require.NoError(t, prepareLoadedProfile(prof))
					rows := prof.Definition.Metrics
					if section == "topology" {
						require.Len(t, prof.Definition.Topology, 1)
						rows = []ddprofiledefinition.MetricsConfig{prof.Definition.Topology[0].MetricsConfig}
					}
					require.Len(t, rows, 1)
					assert.Equal(t, "example", rows[0].Symbol.Name)
					assert.Equal(t, "1.2.3.0", rows[0].Symbol.OID)
					assert.Equal(t, tc.mib, rows[0].MIB)
					assert.Empty(t, rows[0].Name)
					assert.Empty(t, rows[0].OID)
				})
			}
		})
	}
}

func TestLoadProfile_IgnoresRetiredConstantField(t *testing.T) {
	for name, tc := range map[string]struct {
		base      string
		raw       string
		want      string
		wantError bool
	}{
		"scalar with OID": {
			raw:  `metrics: [{symbol: {OID: 1.2.3.0, name: real, constant_value_one: true}}]`,
			want: `metrics: [{symbol: {OID: 1.2.3.0, name: real}}]`,
		},
		"table with OID": {
			raw:  `metrics: [{table: {OID: 1.2.3, name: example}, symbols: [{OID: 1.2.3.1, name: real, constant_value_one: true}], metric_tags: [{index: 1}]}]`,
			want: `metrics: [{table: {OID: 1.2.3, name: example}, symbols: [{OID: 1.2.3.1, name: real}], metric_tags: [{index: 1}]}]`,
		},
		"false flag": {
			raw:  `metrics: [{symbol: {OID: 1.2.3.0, name: real, constant_value_one: false}}]`,
			want: `metrics: [{symbol: {OID: 1.2.3.0, name: real}}]`,
		},
		"uninterpreted flag value": {
			raw:  `metrics: [{symbol: {OID: 1.2.3.0, name: real, constant_value_one: {ignored: value}}}]`,
			want: `metrics: [{symbol: {OID: 1.2.3.0, name: real}}]`,
		},
		"OID-less column": {
			raw:       `metrics: [{table: {OID: 1.2.3, name: example}, symbols: [{name: presence, constant_value_one: true}], metric_tags: [{index: 1}]}]`,
			wantError: true,
		},
		"OID-less column in mixed table": {
			raw:       `metrics: [{table: {OID: 1.2.3, name: example}, symbols: [{name: presence, constant_value_one: true}, {OID: 1.2.3.1, name: real}], metric_tags: [{index: 1}]}]`,
			wantError: true,
		},
		"child symbol remains an ordinary override": {
			base: `metrics: [{table: {OID: 1.2.3, name: example}, symbols: [{OID: 1.2.3.1, name: real}], metric_tags: [{index: 1}]}]`,
			raw: `extends: [base.yaml]
metrics: [{table: {OID: 1.2.3, name: example}, symbols: [{OID: 1.2.3.2, name: real, constant_value_one: true}], metric_tags: [{index: 1}]}]`,
			want: `extends: [base.yaml]
metrics: [{table: {OID: 1.2.3, name: example}, symbols: [{OID: 1.2.3.2, name: real}], metric_tags: [{index: 1}]}]`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			load := func(name, raw string) (*Profile, error) {
				t.Helper()
				file := filepath.Join(dir, name)
				require.NoError(t, os.WriteFile(file, []byte(raw), 0600))
				profile, err := loadProfile(file, multipath.New(dir))
				if err != nil {
					return nil, err
				}
				return profile, prepareLoadedProfile(profile)
			}
			if tc.base != "" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "base.yaml"), []byte(tc.base), 0600))
			}
			got, err := load("profile.yaml", tc.raw)
			if tc.wantError {
				require.ErrorContains(t, err, "symbol oid missing")
				return
			}
			require.NoError(t, err)
			want, err := load("expected.yaml", tc.want)
			require.NoError(t, err)
			assert.Equal(t, want.Definition, got.Definition)
		})
	}
}

func TestCatalog_ProjectSysobjectIDMetadataWithoutOrdinaryMetadata(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "profile.yaml")
	require.NoError(t, os.WriteFile(file, []byte(`selector:
  - sysobjectid: {include: ["1.2.3"]}
metadata:
  device:
    fields:
      vendor: {value: example, consumers: [bgp]}
sysobjectid_metadata:
  - sysobjectid: "1.2.3"
    metadata:
      model: {value: model, consumers: [metrics]}
`), 0600))
	prof, err := loadProfile(file, multipath.New(dir))
	require.NoError(t, err)
	require.NoError(t, prepareLoadedProfile(prof))
	catalog := &Catalog{
		profiles: []*Profile{prof},
	}
	profiles := catalog.Resolve(ResolveRequest{
		SysObjectID: "1.2.3",
	}).Project(ConsumerMetrics).Profiles()
	require.Len(t, profiles, 1)
	assert.Empty(t, profiles[0].Definition.Metadata)
	require.Len(t, profiles[0].Definition.SysobjectIDMetadata, 1)
	assert.Equal(t, "model", profiles[0].Definition.SysobjectIDMetadata[0].Metadata["model"].Value)
}
