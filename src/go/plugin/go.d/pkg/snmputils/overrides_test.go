package snmputils

import (
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
				EnterpriseNumbers: enterpriseNumbersOverrides{OrgToVendor: map[string]string{}},
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
		tc := tc
		t.Run(name, func(t *testing.T) {
			defer withOverrides(t, tc.overrides)()

			si := &SysInfo{SysObjectID: tc.oid}
			updateMetadata(si)

			assert.Equal(t, tc.wantCat, si.Category)
			assert.Equal(t, tc.wantModel, si.Model)
		})
	}
}

func TestOrgToVendorMapping(t *testing.T) {
	cases := map[string]struct {
		oid string
	}{
		"maps_org_to_vendor_when_override_present": {oid: "1.3.6.1.4.1.2505.3"},
	}

	for name, tc := range cases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			rawOrg := lookupEnterpriseNumber(tc.oid)
			if rawOrg == "" {
				t.Skip("no organization resolved for test OID; skipping")
			}

			defer withOverrides(t, &overrides{
				EnterpriseNumbers: enterpriseNumbersOverrides{
					OrgToVendor: map[string]string{
						rawOrg: "CanonicalVendor",
					},
				},
				SysObjectIDs: sysObjectIDOverrides{
					OIDOverrides: map[string]sysObjectIDOverride{},
					CategoryMap:  map[string]string{},
				},
			})()

			si := &SysInfo{SysObjectID: tc.oid}
			updateMetadata(si)

			assert.Equal(t, "CanonicalVendor", si.Vendor, "vendor mapping via org_to_vendor failed")
		})
	}
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
		tc := tc
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
		tc := tc
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
