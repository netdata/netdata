// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/stretchr/testify/require"
)

func TestLoadProfile_StockDefinitions(t *testing.T) {
	root := "../../../config/go.d/snmp.profiles"
	files, err := filepath.Glob(filepath.Join(root, "default", "*.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	cases := make(map[string]struct{ path string }, len(files))
	for _, file := range files {
		cases[filepath.Base(file)] = struct{ path string }{file}
	}
	paths := multipath.MultiPath{filepath.Join(root, "default"), filepath.Join(root, "metadata")}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			profile, err := loadProfile(tc.path, paths)
			require.NoError(t, err)
			// Directory discovery logs and skips invalid profiles; check every concrete and abstract profile directly.
			require.NoError(t, prepareLoadedProfile(profile))
		})
	}
}
