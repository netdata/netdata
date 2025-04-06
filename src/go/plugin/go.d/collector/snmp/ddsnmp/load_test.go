// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_loadDDSnmpProfiles(t *testing.T) {
	dir, _ := filepath.Abs("../../../config/go.d/snmp.profiles/default")

	f, err := os.Open(dir)
	require.NoError(t, err)
	defer f.Close()

	profiles, err := load(dir)
	require.NoError(t, err)

	names, err := f.Readdirnames(-1)
	require.NoError(t, err)

	require.Equal(t, len(names)-1 /*README.md*/, len(profiles))
}
