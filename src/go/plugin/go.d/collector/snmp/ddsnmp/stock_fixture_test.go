// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
)

// Share concrete-stock setup without replacing the independent default catalog or
// the direct all-definition, raw YAML and effective accessibility checks.
var loadStockProfilesForTest = sync.OnceValues(func() ([]*Profile, error) {
	dir, err := filepath.Abs("../../../config/go.d/snmp.profiles/default")
	if err != nil {
		return nil, err
	}
	return loadProfilesFromDir(dir, multipath.New(dir))
})

func stockProfilesForTest() ([]*Profile, error) {
	profiles, err := loadStockProfilesForTest()
	if err != nil {
		return nil, err
	}

	// Each caller owns its profiles, including nested slices and maps.
	cloned := make([]*Profile, len(profiles))
	for i, profile := range profiles {
		cloned[i] = profile.clone()
	}
	return cloned, nil
}

func TestStockProfilesForTestOwnsReturnedProfiles(t *testing.T) {
	first, err := stockProfilesForTest()
	require.NoError(t, err)
	index := slices.IndexFunc(first, func(profile *Profile) bool {
		return len(profile.Definition.Metrics) > 0
	})
	require.NotEqual(t, -1, index)
	file := first[index].SourceFile
	metric := first[index].Definition.Metrics[0].Symbol.Name
	first[index].SourceFile = "changed by caller"
	first[index].Definition.Metrics[0].Symbol.Name = "changed by caller"

	second, err := stockProfilesForTest()
	require.NoError(t, err)
	require.Equal(t, file, second[index].SourceFile)
	require.Equal(t, metric, second[index].Definition.Metrics[0].Symbol.Name)
}
