// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/profiletest"
)

func writeRootTestProfileMetricProfile(t testing.TB, dir string) {
	t.Helper()
	profiletest.WriteCiscoConfigMetricProfile(t, dir)
}

func rootTestProfileCatalogPaths(t testing.TB) catalog.Paths {
	t.Helper()
	return profiletest.CatalogPaths(t)
}
