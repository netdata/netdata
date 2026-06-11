package snmputils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withOverrides(t *testing.T, o *overrides) func() {
	t.Helper()
	prev := overridesData
	overridesData = o
	return func() { overridesData = prev }
}

func TestOverrides_OnUnknownOID(t *testing.T) {
	cases := map[string]struct {
		oid       string
		overrides *overrides
		wantCat   string
		wantModel string
	}{
		"create_from_override_with_normalization": {
			oid: "1.3.6.1.4.1.99999.42", // not in base DB
			overrides: &overrides{
				SysObjectIDs: sysObjectIDOverrides{
					OIDOverrides: map[string]sysObjectIDOverride{
						"1.3.6.1.4.1.99999.42": {
							Category: "UTM",     // will normalize
							Model:    "XR-1000", // set model
						},
					},
					CategoryMap: map[string]string{
						"UTM": "Firewall",
					},
				},
			},
			wantCat:   "Firewall",
			wantModel: "XR-1000",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			defer withOverrides(t, tc.overrides)()

			si := &SysInfo{SysObjectID: tc.oid}
			updateMetadata(si)

			assert.Equal(t, tc.wantCat, si.Category)
			assert.Equal(t, tc.wantModel, si.Model)
		})
	}
}

func TestPenToVendorMappingDoesNotRequireRegistry(t *testing.T) {
	withEnterpriseNumbersFile(t, filepath.Join(t.TempDir(), "missing.txt"))
	defer withOverrides(t, &overrides{
		EnterpriseNumbers: enterpriseNumbersOverrides{
			PenToVendor: map[string]string{
				"2505": "CanonicalVendor",
			},
		},
	})()

	si := &SysInfo{SysObjectID: "1.3.6.1.4.1.2505.3"}
	updateMetadata(si)

	assert.Equal(t, "CanonicalVendor", si.Vendor, "vendor mapping via pen_to_vendor failed")
	assert.Equal(t, "Unknown", si.Organization)
}

func TestPenToVendorMappingTakesPrecedenceOverOrgToVendor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "iana-enterprise-numbers.txt")
	require.NoError(t, os.WriteFile(path, []byte(`
424242
  Example Devices Inc.
`), 0644))
	withEnterpriseNumbersFile(t, path)
	defer withOverrides(t, &overrides{
		EnterpriseNumbers: enterpriseNumbersOverrides{
			PenToVendor: map[string]string{
				"424242": "VendorByPEN",
			},
			OrgToVendor: map[string]string{
				"Example Devices Inc.": "VendorByOrg",
			},
		},
	})()

	si := &SysInfo{SysObjectID: "1.3.6.1.4.1.424242.3"}
	updateMetadata(si)

	assert.Equal(t, "VendorByPEN", si.Vendor)
}

func TestOrgToVendorMappingFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "iana-enterprise-numbers.txt")
	require.NoError(t, os.WriteFile(path, []byte(`
424242
  Example Devices Inc.
`), 0644))
	withEnterpriseNumbersFile(t, path)
	defer withOverrides(t, &overrides{
		EnterpriseNumbers: enterpriseNumbersOverrides{
			OrgToVendor: map[string]string{
				"Example Devices Inc.": "VendorByOrg",
			},
		},
	})()

	si := &SysInfo{SysObjectID: "1.3.6.1.4.1.424242.3"}
	updateMetadata(si)

	assert.Equal(t, "VendorByOrg", si.Vendor)
}

func TestLookupEnterpriseNumber(t *testing.T) {
	cases := map[string]struct {
		oid          string
		wantNonEmpty bool
	}{
		"known_pen_should_resolve_org":       {oid: "1.3.6.1.4.1.2505.3", wantNonEmpty: true},
		"not_under_enterprise_returns_empty": {oid: "1.3.6.1.2.1.1.2.0", wantNonEmpty: false},
		"too_short_returns_empty":            {oid: "1.3.6.1.4.1", wantNonEmpty: false},
		"trailing_dot_returns_empty":         {oid: "1.3.6.1.4.1.", wantNonEmpty: false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := lookupEnterpriseNumber(tc.oid)
			if tc.wantNonEmpty {
				require.NotEmpty(t, got, "expected non-empty org for oid %s", tc.oid)
			} else {
				assert.Empty(t, got, "expected empty org for oid %s, but got %s", tc.oid, got)
			}
		})
	}
}

func TestLookupEnterpriseNumberLoadsRegistryFromDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "iana-enterprise-numbers.txt")
	require.NoError(t, os.WriteFile(path, []byte(`
424242
  Example Devices Inc.
    Contact
424243
  Reserved
424244
  ---none---
424245
  Unassigned
424246
  Other Devices Inc.
`), 0644))
	withEnterpriseNumbersFile(t, path)

	assert.Equal(t, "Example Devices Inc.", lookupEnterpriseNumber("1.3.6.1.4.1.424242.1"))
	assert.Empty(t, lookupEnterpriseNumber("1.3.6.1.4.1.424243.1"))
	assert.Empty(t, lookupEnterpriseNumber("1.3.6.1.4.1.424244.1"))
	assert.Empty(t, lookupEnterpriseNumber("1.3.6.1.4.1.424245.1"))
	assert.Equal(t, "Other Devices Inc.", lookupEnterpriseNumber("1.3.6.1.4.1.424246.1"))
}

func TestLookupEnterpriseNumberSkipsDuplicateRegistryEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "iana-enterprise-numbers.txt")
	require.NoError(t, os.WriteFile(path, []byte(`
424242
  Example Devices Inc.
424243
  Other Devices Inc.
424242
  Duplicate Devices Inc.
`), 0644))
	withEnterpriseNumbersFile(t, path)

	assert.Equal(t, "Example Devices Inc.", lookupEnterpriseNumber("1.3.6.1.4.1.424242.1"))
	assert.Equal(t, "Other Devices Inc.", lookupEnterpriseNumber("1.3.6.1.4.1.424243.1"))
}

func TestEnterpriseNumbersFilePathFindsSourceTreeRegistry(t *testing.T) {
	oldPath := enterpriseNumbersPathOverride
	enterpriseNumbersPathOverride = ""
	t.Cleanup(func() {
		enterpriseNumbersPathOverride = oldPath
	})

	path := enterpriseNumbersFilePath()

	require.FileExists(t, path)
	assert.Contains(t, filepath.ToSlash(path), "plugin/go.d/config/go.d/snmp.profiles/metadata/iana-enterprise-numbers.txt")
}

func TestLookupEnterpriseNumberMissingRegistryReturnsEmpty(t *testing.T) {
	withEnterpriseNumbersFile(t, filepath.Join(t.TempDir(), "missing.txt"))

	assert.Empty(t, lookupEnterpriseNumber("1.3.6.1.4.1.424242.1"))
}

func TestLookupEnterpriseNumberRetriesAfterMissingRegistry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "iana-enterprise-numbers.txt")
	withEnterpriseNumbersFile(t, path)

	assert.Empty(t, lookupEnterpriseNumber("1.3.6.1.4.1.424242.1"))

	require.NoError(t, os.WriteFile(path, []byte(`
424242
  Example Devices Inc.
`), 0644))

	assert.Equal(t, "Example Devices Inc.", lookupEnterpriseNumber("1.3.6.1.4.1.424242.1"))
}

func TestLookupEnterpriseNumberNonEnterpriseOIDDoesNotLoadRegistry(t *testing.T) {
	withEnterpriseNumbersFile(t, filepath.Join(t.TempDir(), "missing.txt"))

	assert.Empty(t, lookupEnterpriseNumber("1.3.6.1.2.1.1.2.0"))
	assert.Nil(t, enterpriseNumbers.values)
	assert.NoError(t, enterpriseNumbers.err)
}

func withEnterpriseNumbersFile(t *testing.T, path string) {
	t.Helper()

	oldPath := enterpriseNumbersPathOverride
	enterpriseNumbersPathOverride = path
	enterpriseNumbers = enterpriseNumbersCache{}
	t.Cleanup(func() {
		enterpriseNumbersPathOverride = oldPath
		enterpriseNumbers = enterpriseNumbersCache{}
	})
}

func TestPduToString(t *testing.T) {
	cases := map[string]struct {
		pdu     gosnmp.SnmpPDU
		want    string
		wantErr bool
	}{
		"octet_string": {
			pdu:  gosnmp.SnmpPDU{Type: gosnmp.OctetString, Value: []byte("hello\nworld")},
			want: "hello\nworld",
		},
		"integer": {
			pdu:  gosnmp.SnmpPDU{Type: gosnmp.Integer, Value: int(42)},
			want: "42",
		},
		"counter32": {
			pdu:  gosnmp.SnmpPDU{Type: gosnmp.Counter32, Value: uint32(7)},
			want: "7",
		},
		"object_identifier_trims_dot": {
			pdu:  gosnmp.SnmpPDU{Type: gosnmp.ObjectIdentifier, Value: ".1.3.6.1.2.1.1.1.0"},
			want: "1.3.6.1.2.1.1.1.0",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := PduToString(tc.pdu)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestAllMetadataYAMLsLoadAndMerge(t *testing.T) {
	dir := getSnmpMetadataDir()
	require.NotEmpty(t, dir, "metadata dir must resolve in tests")

	agg, err := loadOverridesFromDir(dir)
	require.NoError(t, err, "every YAML must parse strictly without errors")
	require.NotNil(t, agg, "aggregate overrides must not be nil")
}

func TestShippedPenToVendorOverridesCoverCurrentPENRenames(t *testing.T) {
	dir := getSnmpMetadataDir()
	require.NotEmpty(t, dir, "metadata dir must resolve in tests")

	agg, err := loadOverridesFromDir(dir)
	require.NoError(t, err)
	require.Equal(t, "Nokia", agg.EnterpriseNumbers.PenToVendor["6486"])
	require.Equal(t, "Infinera", agg.EnterpriseNumbers.PenToVendor["21296"])
	require.Equal(t, "NEC", agg.EnterpriseNumbers.PenToVendor["48079"])

	withEnterpriseNumbersFile(t, enterpriseNumbersFilePath())
	rawOrg := lookupEnterpriseNumber("1.3.6.1.4.1.21296.1")
	require.Equal(t, "Nokia (formerly 'Infinera Corp.')", rawOrg)

	defer withOverrides(t, agg)()
	si := &SysInfo{SysObjectID: "1.3.6.1.4.1.21296.1"}
	updateMetadata(si)

	assert.Equal(t, "Infinera", si.Vendor)
}
