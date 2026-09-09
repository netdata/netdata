// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustLoadTypedLicensingProfile(t *testing.T, profileName string, keep func(row ddprofiledefinition.LicensingConfig) bool) *ddsnmp.Profile {
	t.Helper()

	profiles := ddsnmp.DefaultCatalog().Resolve(ddsnmp.ResolveRequest{
		ManualProfiles: []string{profileName},
		ManualPolicy:   ddsnmp.ManualProfileFallback,
	}).Project(ddsnmp.ConsumerLicensing).Profiles()
	require.Len(t, profiles, 1)

	profile := profiles[0]
	profile.Definition.Metadata = nil
	profile.Definition.SysobjectIDMetadata = nil
	profile.Definition.MetricTags = nil
	profile.Definition.StaticTags = nil
	profile.Definition.VirtualMetrics = nil
	profile.Definition.Topology = nil
	profile.Definition.Metrics = nil
	profile.Definition.Licensing = slices.DeleteFunc(profile.Definition.Licensing, func(row ddprofiledefinition.LicensingConfig) bool {
		return !keep(row)
	})

	require.NotEmpty(t, profile.Definition.Licensing)
	assert.Equal(t, profileName+".yaml", filepath.Base(profile.SourceFile))

	return profile
}

func licenseRowsByID(rows []ddsnmp.LicenseRow) map[string]ddsnmp.LicenseRow {
	out := make(map[string]ddsnmp.LicenseRow, len(rows))
	for _, row := range rows {
		out[row.ID] = row
	}
	return out
}

func hasLicensingTable(profile *ddsnmp.Profile, oid string) bool {
	for _, row := range profile.Definition.Licensing {
		if strings.TrimPrefix(row.Table.OID, ".") == oid {
			return true
		}
	}
	return false
}
