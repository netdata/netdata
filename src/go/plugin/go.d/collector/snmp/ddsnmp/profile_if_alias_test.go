// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	oidIfName  = "1.3.6.1.2.1.31.1.1.1.1"
	oidIfAlias = "1.3.6.1.2.1.31.1.1.1.18"
)

// Every chart identified by ifName must also carry the operator-assigned alias as the `if_alias` label,
// read from the same row scope, so the UI can display it consistently across profiles.
func TestStockProfiles_IfNameInterfaceTagsCarryIfAlias(t *testing.T) {
	profiles, err := stockProfilesForTest()
	require.NoError(t, err)
	require.NotEmpty(t, profiles)

	var checked int
	for _, prof := range profiles {
		for _, m := range prof.Definition.Metrics {
			for _, tag := range m.MetricTags {
				if (tag.Tag != "interface" && tag.Tag != "_interface") || tag.Symbol.OID != oidIfName {
					continue
				}
				checked++

				want := tag.Clone()
				want.Tag = "_if_alias"
				want.Symbol.OID = oidIfAlias
				want.Symbol.Name = "ifAlias"

				assert.Containsf(t, m.MetricTags, want,
					"profile %s table %s: ifName interface tag has no matching _if_alias tag",
					filepath.Base(prof.SourceFile), m.Table.Name)
			}
		}
	}
	assert.NotZero(t, checked, "no ifName interface tags found in stock profiles")
}
