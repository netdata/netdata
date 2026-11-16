// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestPollingAccessibility_EffectiveExceptionScope(t *testing.T) {
	manifest := readAccessManifest(t)
	facts := map[string]profileAccessFact{}
	for _, fact := range manifest.Facts {
		facts[accessFactKey(fact.Declared, fact.OID)] = fact
	}
	const role = "topology-role-mikrotik-rb750gr3.yaml"
	content, err := os.ReadFile("../../../config/go.d/snmp.profiles/default/" + role)
	require.NoError(t, err)
	selector := func(oid string) any {
		return []any{map[string]any{"sysobjectid": map[string]any{"include": []string{oid}}}}
	}
	for name, tc := range map[string]struct {
		file                  string
		selector              any
		transitive            bool
		wantProblems          int
		objectID, description string
	}{
		"original role": {
			file:        role,
			selector:    manifest.ExceptionSelector,
			objectID:    "1.3.6.1.4.1.14988.1",
			description: "RouterOS RB750Gr3",
		},
		"same proven selector inherited": {
			file:        "matched-child.yaml",
			selector:    manifest.ExceptionSelector,
			objectID:    "1.3.6.1.4.1.14988.1",
			description: "RouterOS RB750Gr3",
		},
		"generic direct inheritance": {
			file:         "generic-device.yaml",
			selector:     selector(".*"),
			wantProblems: 3,
			objectID:     "1.3.6.1.4.1.9.1.1",
			description:  "Cisco device",
		},
		"generic transitive inheritance": {
			file:         "broad-child.yaml",
			selector:     selector(".*"),
			transitive:   true,
			wantProblems: 3,
			objectID:     "1.3.6.1.4.1.9.1.1",
			description:  "Cisco device",
		},
		"different selector direct inheritance": {
			file:         "different-child.yaml",
			selector:     selector("1.3.6.1.4.1.9.1.1"),
			wantProblems: 3,
			objectID:     "1.3.6.1.4.1.9.1.1",
			description:  "Cisco device",
		},
		"vendor direct inheritance": {
			file:         "vendor-family.yaml",
			selector:     selector(`^1\.3\.6\.1\.4\.1\.14988\.`),
			wantProblems: 3,
			objectID:     "1.3.6.1.4.1.14988.9",
			description:  "RouterOS other model",
		},
		"different selector transitive inheritance": {
			file:         "unrelated-child.yaml",
			selector:     selector("1.3.6.1.4.1.9.1.1"),
			transitive:   true,
			wantProblems: 3,
			objectID:     "1.3.6.1.4.1.9.1.1",
			description:  "Cisco device",
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, role), content, 0600))
			if tc.file != role {
				parent := role
				if tc.transitive {
					parent = "_intermediate.yaml"
					data, err := yaml.Marshal(map[string]any{"extends": []string{role}})
					require.NoError(t, err)
					require.NoError(t, os.WriteFile(filepath.Join(dir, parent), data, 0600))
				}
				data, err := yaml.Marshal(map[string]any{"extends": []string{parent}, "selector": tc.selector})
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(dir, tc.file), data, 0600))
			}
			profile, err := loadProfile(filepath.Join(dir, tc.file), multipath.New(dir))
			require.NoError(t, err)
			require.NoError(t, prepareLoadedProfile(profile))
			matched, _ := profile.Definition.Selector.Matches(tc.objectID, tc.description)
			require.True(t, matched, "production selector must expose the effective acquisition rows")
			require.Len(t, profile.Definition.Topology, 1)
			row := profile.Definition.Topology[0]
			require.Equal(t, "1.0.8802.1.1.2.1.4.2.1.1", row.Symbols[0].OID)
			require.Equal(t, "1.0.8802.1.1.2.1.4.2.1.1", row.MetricTags[2].Symbol.OID)
			require.Equal(t, "1.0.8802.1.1.2.1.4.2.1.2", row.MetricTags[4].Symbol.OID)
			problems, err := effectivePollingScopeProblems(profile, facts, manifest.ExceptionSelector)
			require.NoError(t, err)
			require.Len(t, problems, tc.wantProblems)
		})
	}
}
