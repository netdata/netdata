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
				return fmt.Sprintf("%s:\n  - symbol: {OID: 1.2.3.0, name: example}\n%s    MIB: %s\n", section, kind, description)
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

func TestLoadProfile_RemovesConstantColumns(t *testing.T) {
	for name, tc := range map[string]struct {
		extra    string
		wantRows int
	}{
		"all constant": {wantRows: 0},
		"mixed":        {extra: "      - {OID: 1.2.3.1, name: real}\n", wantRows: 1},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			raw := "metrics:\n  - table: {OID: 1.2.3, name: example}\n    symbols:\n      - {name: presence, constant_value_one: true}\n" + tc.extra + "    metric_tags: [{tag: index, index: 1}]\n"
			file := filepath.Join(dir, "profile.yaml")
			require.NoError(t, os.WriteFile(file, []byte(raw), 0600))
			prof, err := loadProfile(file, multipath.New(dir))
			require.NoError(t, err)
			require.NoError(t, prepareLoadedProfile(prof))
			require.Len(t, prof.Definition.Metrics, tc.wantRows)
			if tc.wantRows > 0 {
				require.Len(t, prof.Definition.Metrics[0].Symbols, 1)
				assert.Equal(t, "real", prof.Definition.Metrics[0].Symbols[0].Name)
			}
			require.NoError(t, ddprofiledefinition.ValidateEnrichProfile(prof.Definition))
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
