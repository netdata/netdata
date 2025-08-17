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

func TestBaseMeta_FromEmbeddedDB(t *testing.T) {
	cases := map[string]struct {
		oid       string
		wantCat   string
		wantModel string
	}{
		"hp_vc_40gb_module": {
			oid:       "1.3.6.1.2.1.11.5.7.5.8",
			wantCat:   "SANSwitch",
			wantModel: "Virtual Connect SE 40Gb F8 Module",
		},
		"hp_vc_100gb_module": {
			oid:       "1.3.6.1.2.1.11.5.7.5.9",
			wantCat:   "SANSwitch",
			wantModel: "Virtual Connect SE 100Gb F32 Module",
		},
		"abb_ups_dpa": {
			oid:       "1.3.6.1.2.1.33",
			wantCat:   "UPS",
			wantModel: "DPA",
		},
		"nx_gtx_1000": {
			oid:       "1.3.6.1.4.1.1.1.1.66",
			wantCat:   "Router",
			wantModel: "GTX 1000",
		},
		"nx_vpn_router_1000": {
			oid:       "1.3.6.1.4.1.2505.3",
			wantCat:   "Firewall",
			wantModel: "Nortel Networks VPN Router 1000",
		},
	}

	for name, tc := range cases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			defer withOverrides(t, nil)() // ensure no YAML overrides

			si := &SysInfo{SysObjectID: tc.oid}
			updateMetadata(si)

			assert.Equal(t, tc.wantCat, si.Category, "category mismatch for %s", tc.oid)
			assert.Equal(t, tc.wantModel, si.Model, "model mismatch for %s", tc.oid)
		})
	}
}

func TestOverrides_OnExistingOID(t *testing.T) {
	cases := map[string]struct {
		oid       string
		overrides *overrides
		wantCat   string
		wantModel string
	}{
		"override_then_normalize_category": {
			oid: "1.3.6.1.2.1.11.5.7.5.8", // base: SANSwitch / VC 40Gb
			overrides: &overrides{
				EnterpriseNumbers: enterpriseNumbersOverrides{
					OrgToVendor: map[string]string{},
				},
				SysObjectIDs: sysObjectIDOverrides{
					OIDOverrides: map[string]sysObjectIDOverride{
						"1.3.6.1.2.1.11.5.7.5.8": {
							Category: "L3 Switch", // will normalize
							Model:    "Renamed Model",
						},
					},
					CategoryMap: map[string]string{
						"L3 Switch": "Switch",
					},
				},
			},
			wantCat:   "Switch",
			wantModel: "Renamed Model",
		},
		"override_model_only_keep_base_category": {
			oid: "1.3.6.1.2.1.33", // base: UPS / DPA
			overrides: &overrides{
				EnterpriseNumbers: enterpriseNumbersOverrides{OrgToVendor: map[string]string{}},
				SysObjectIDs: sysObjectIDOverrides{
					OIDOverrides: map[string]sysObjectIDOverride{
						"1.3.6.1.2.1.33": {Model: "DPA-X"},
					},
					CategoryMap: map[string]string{}, // no normalization
				},
			},
			wantCat:   "UPS",
			wantModel: "DPA-X",
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
			assert.Empty(t, si.Vendor, "Vendor should remain empty without org_to_vendor mapping")
		})
	}
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
