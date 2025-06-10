// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_loadDDSnmpProfiles(t *testing.T) {
	dir, _ := filepath.Abs("../../../config/go.d/snmp.profiles/default")

	f, err := os.Open(dir)
	require.NoError(t, err)
	defer f.Close()

	profiles, err := loadProfiles(dir)
	require.NoError(t, err)

	require.NotEmpty(t, profiles)

	names, err := f.Readdirnames(-1)
	require.NoError(t, err)

	require.Equal(t, len(names)-1 /*README.md*/, len(profiles))
}

func Test_FindProfiles(t *testing.T) {
	test := map[string]struct {
		sysObjOId   string
		wanProfiles int
	}{
		"mikrotik": {
			sysObjOId:   "1.3.6.1.4.1.14988.1",
			wanProfiles: 2,
		},
		"no match": {
			sysObjOId:   "0.1.2.3",
			wanProfiles: 0,
		},
	}

	for name, test := range test {
		t.Run(name, func(t *testing.T) {
			profiles := FindProfiles(test.sysObjOId)

			require.Len(t, profiles, test.wanProfiles)
		})
	}
}

func Test_Profile_merge(t *testing.T) {
	profiles := FindProfiles("1.3.6.1.4.1.9.1.1216") // cisco-nexus

	i := slices.IndexFunc(profiles, func(p *Profile) bool {
		return strings.HasSuffix(p.SourceFile, "cisco-nexus.yaml")
	})

	require.GreaterOrEqual(t, 1, 0)

	for _, m := range profiles[i].Definition.Metrics {
		if m.IsColumn() && m.Table.Name == "ciscoMemoryPoolTable" {
			for _, s := range m.Symbols {
				assert.NotEqual(t, "memory.used", s.Name)
			}
		}
	}
}
