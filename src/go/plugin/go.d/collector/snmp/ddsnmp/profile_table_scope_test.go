// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// Stock rows must not read same-table OIDs outside their table.OID: the loader only warns, and such OIDs are never
// collected. Abstract profiles are checked too, including ones no stock profile extends yet.
func TestStockProfiles_TableRowOIDsStayInTable(t *testing.T) {
	dir, err := filepath.Abs("../../../config/go.d/snmp.profiles/default")
	require.NoError(t, err)
	paths := multipath.New(dir)

	var checked int
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".yaml") {
			return err
		}
		profile, err := loadProfile(path, paths)
		require.NoError(t, err, d.Name())
		require.NoError(t, prepareLoadedProfile(profile), d.Name())
		checked++

		assert.Empty(t, ddprofiledefinition.OutOfTableOIDs(profile.Definition), d.Name())
		return nil
	})
	require.NoError(t, err)
	assert.NotZero(t, checked, "no stock profiles found")
}
