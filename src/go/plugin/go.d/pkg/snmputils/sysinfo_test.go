// SPDX-License-Identifier: GPL-3.0-or-later

package snmputils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"testing"

	"github.com/netdata/gosnmp"
	snmpmock "github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils/snmpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGetSysInfoRecordsProbeDiagnostics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pdus := []gosnmp.VarBind{
		{Name: "." + OidSysDescr, Value: gosnmp.OctetStringValue([]byte("network device"))},
		{Name: OidSysObject, Value: gosnmp.ObjectIdentifierValue(".1.3.6.1.4.1.9.1.1166")},
		{Name: OidSysContact, Value: gosnmp.OctetStringValue([]byte("operations"))},
		{Name: "." + OidSysName, Value: gosnmp.OctetStringValue([]byte("router"))},
		{Name: OidSysLocation, Value: gosnmp.OctetStringValue([]byte("datacenter"))},
	}

	client := snmpmock.NewMockSession(ctrl)
	client.EXPECT().Get(gomock.Any(), sysInfoOIDs()).Return(testSysInfoResponse(pdus), nil)

	si, err := GetSysInfo(context.Background(), client, gosnmp.Version2c, len(sysInfoOIDs()))
	require.NoError(t, err)
	require.NotNil(t, si)
	assert.Equal(t, "1.3.6.1.4.1.9.1.1166", si.SysObjectID)
	assert.Equal(t, SysInfoProbe{
		PDUCount:        len(pdus),
		SeenSysDescr:    true,
		SeenSysObjectID: true,
		SysObjectIDType: "ObjectIdentifier",
		SeenSysContact:  true,
		SeenSysName:     true,
		SeenSysLocation: true,
	}, si.Probe)
}

func TestGetSysInfoNormalizesMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := snmpmock.NewMockSession(ctrl)
	const input = "device\u0085a\u2028b\u2029c\x1fd\x7fe\x00'\"`\\"
	var pdus []gosnmp.VarBind
	for _, oid := range []string{OidSysDescr, OidSysContact, OidSysName, OidSysLocation} {
		pdus = append(pdus, gosnmp.VarBind{
			Name:  oid,
			Value: gosnmp.OctetStringValue([]byte(input)),
		})
	}
	client.EXPECT().Get(gomock.Any(), sysInfoOIDs()).Return(testSysInfoResponse(pdus), nil)
	si, err := GetSysInfo(context.Background(), client, gosnmp.Version2c, len(sysInfoOIDs()))
	require.NoError(t, err)
	for name, value := range map[string]string{"description": si.Descr, "contact": si.Contact, "name": si.Name, "location": si.Location} {
		t.Run(name, func(t *testing.T) { assert.Equal(t, "device a b c d e", value) })
	}
}

func TestSanitizeMetadata(t *testing.T) {
	for _, bounds := range [][2]rune{{1, 0x1f}, {0x7f, 0x9f}, {0x2028, 0x2029}} {
		for r := bounds[0]; r <= bounds[1]; r++ {
			t.Run(fmt.Sprintf("U+%04X", r), func(t *testing.T) {
				assert.Equal(t, "left right", sanitizeMetadata("left"+string(r)+"right"))
			})
		}
	}
	for name, test := range map[string]struct{ input, want string }{
		"legacy removals":  {input: "a'\"`\\\x00b", want: "ab"},
		"ordinary Unicode": {input: "router-é 路由器: west #1", want: "router-é 路由器: west #1"},
		"empty":            {input: "", want: ""},
	} {
		t.Run(name, func(t *testing.T) { assert.Equal(t, test.want, sanitizeMetadata(test.input)) })
	}
}

func TestGetSysInfoChunksRequestsByMaxOids(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := snmpmock.NewMockSession(ctrl)
	gomock.InOrder(
		client.EXPECT().Get(gomock.Any(), []string{OidSysDescr, OidSysObject}).Return(testSysInfoResponse([]gosnmp.VarBind{
			{Name: OidSysDescr, Value: gosnmp.OctetStringValue([]byte("network device"))},
			{Name: OidSysObject, Value: gosnmp.ObjectIdentifierValue(".1.3.6.1.4.1.9.1.1166")},
		}), nil),
		client.EXPECT().Get(gomock.Any(), []string{OidSysContact, OidSysName}).Return(testSysInfoResponse([]gosnmp.VarBind{
			{Name: OidSysContact, Value: gosnmp.OctetStringValue([]byte("operations"))},
			{Name: OidSysName, Value: gosnmp.OctetStringValue([]byte("router"))},
		}), nil),
		client.EXPECT().Get(gomock.Any(), []string{OidSysLocation}).Return(testSysInfoResponse([]gosnmp.VarBind{
			{Name: OidSysLocation, Value: gosnmp.OctetStringValue([]byte("datacenter"))},
		}), nil),
	)

	si, err := GetSysInfo(context.Background(), client, gosnmp.Version2c, 2)
	require.NoError(t, err)
	require.NotNil(t, si)
	assert.Equal(t, "1.3.6.1.4.1.9.1.1166", si.SysObjectID)
	assert.Equal(t, 5, si.Probe.PDUCount)
}

func TestGetSysInfoRejectsInvalidMaxOids(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := snmpmock.NewMockSession(ctrl)

	si, err := GetSysInfo(context.Background(), client, gosnmp.Version2c, 0)
	require.Error(t, err)
	assert.Nil(t, si)
	assert.Contains(t, err.Error(), "invalid maximum OIDs per request 0")
}

func TestGetSysInfoReturnsPartialProbeWithoutIdentityError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := snmpmock.NewMockSession(ctrl)
	client.EXPECT().Get(gomock.Any(), sysInfoOIDs()).Return(testSysInfoResponse([]gosnmp.VarBind{
		{Name: OidSysDescr, Value: gosnmp.OctetStringValue([]byte("network device"))},
		{Name: OidSysName, Value: gosnmp.OctetStringValue([]byte("router"))},
	}), nil)

	si, err := GetSysInfo(context.Background(), client, gosnmp.Version2c, len(sysInfoOIDs()))
	require.NoError(t, err)
	require.NotNil(t, si)
	assert.Empty(t, si.SysObjectID)
	assert.Equal(t, SysInfoProbe{
		PDUCount:     2,
		SeenSysDescr: true,
		SeenSysName:  true,
	}, si.Probe)
}

func TestGetSysInfoReturnsEmptyProbeWithoutIdentityError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := snmpmock.NewMockSession(ctrl)
	client.EXPECT().Get(gomock.Any(), sysInfoOIDs()).Return(testSysInfoResponse(nil), nil)

	si, err := GetSysInfo(context.Background(), client, gosnmp.Version2c, len(sysInfoOIDs()))
	require.NoError(t, err)
	require.NotNil(t, si)
	assert.Empty(t, si.SysObjectID)
	assert.Equal(t, SysInfoProbe{}, si.Probe)
}

func TestGetSysInfoWrapsGetError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	getErr := errors.New("timeout")
	client := snmpmock.NewMockSession(ctrl)
	client.EXPECT().Get(gomock.Any(), sysInfoOIDs()).Return(nil, getErr)

	si, err := GetSysInfo(context.Background(), client, gosnmp.Version2c, len(sysInfoOIDs()))
	require.Error(t, err)
	assert.Nil(t, si)
	assert.ErrorIs(t, err, getErr)
	assert.Contains(t, err.Error(), "SNMP system scalars")
}

func TestGetSysInfoSysObjectIDTypeHandling(t *testing.T) {
	tests := map[string]struct {
		pdu           gosnmp.VarBind
		wantSysObject string
		wantType      string
	}{
		"object identifier": {
			pdu:           gosnmp.VarBind{Value: gosnmp.ObjectIdentifierValue(".1.3.6.1.4.1.11.2.3.9.1")},
			wantSysObject: "1.3.6.1.4.1.11.2.3.9.1",
			wantType:      "ObjectIdentifier",
		},
		"octet string with numeric OID": {
			pdu:           gosnmp.VarBind{Value: gosnmp.OctetStringValue([]byte("1.3.6.1.4.1.11.2.3.9.1"))},
			wantSysObject: "1.3.6.1.4.1.11.2.3.9.1",
			wantType:      "OctetString",
		},
		"octet string with dotted numeric OID and whitespace": {
			pdu:           gosnmp.VarBind{Value: gosnmp.OctetStringValue([]byte(" .1.3.6.1.4.1.11.2.3.9.1\n"))},
			wantSysObject: "1.3.6.1.4.1.11.2.3.9.1",
			wantType:      "OctetString",
		},
		"octet string with NUL-terminated numeric OID": {
			pdu:           gosnmp.VarBind{Value: gosnmp.OctetStringValue([]byte("1.3.6.1.4.1.11.2.3.9.1\x00"))},
			wantSysObject: "1.3.6.1.4.1.11.2.3.9.1",
			wantType:      "OctetString",
		},
		"octet string with joint-iso-itu-t OID": {
			pdu:           gosnmp.VarBind{Value: gosnmp.OctetStringValue([]byte("2.999"))},
			wantSysObject: "2.999",
			wantType:      "OctetString",
		},
		"octet string with IPv4 address": {
			pdu:      gosnmp.VarBind{Value: gosnmp.OctetStringValue([]byte("192.0.2.10"))},
			wantType: "OctetString",
		},
		"octet string with first arc above 2": {
			pdu:      gosnmp.VarBind{Value: gosnmp.OctetStringValue([]byte("3.6"))},
			wantType: "OctetString",
		},
		"octet string with second arc 40 under iso": {
			pdu:      gosnmp.VarBind{Value: gosnmp.OctetStringValue([]byte("1.40"))},
			wantType: "OctetString",
		},
		"octet string with second arc above 39 under itu-t": {
			pdu:      gosnmp.VarBind{Value: gosnmp.OctetStringValue([]byte("0.999"))},
			wantType: "OctetString",
		},
		"octet string with free text": {
			pdu:      gosnmp.VarBind{Value: gosnmp.OctetStringValue([]byte("private device identifier"))},
			wantType: "OctetString",
		},
		"octet string with symbolic OID": {
			pdu:      gosnmp.VarBind{Value: gosnmp.OctetStringValue([]byte("SNMPv2-SMI::enterprises.11.2.3.9.1"))},
			wantType: "OctetString",
		},
		"octet string with single arc": {
			pdu:      gosnmp.VarBind{Value: gosnmp.OctetStringValue([]byte("11"))},
			wantType: "OctetString",
		},
		"octet string with empty arc": {
			pdu:      gosnmp.VarBind{Value: gosnmp.OctetStringValue([]byte("1.3..6.1"))},
			wantType: "OctetString",
		},
		"empty octet string": {
			pdu:      gosnmp.VarBind{Value: gosnmp.OctetStringValue([]byte{})},
			wantType: "OctetString",
		},
		"integer": {
			pdu:      gosnmp.VarBind{Value: gosnmp.IntegerValue(11)},
			wantType: "Integer",
		},
		"unsupported type": {
			pdu:      gosnmp.VarBind{Value: gosnmp.IPAddressValue(netip.MustParseAddr("192.0.2.1"))},
			wantType: "IPAddress",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			pdu := tc.pdu
			pdu.Name = OidSysObject
			client := snmpmock.NewMockSession(ctrl)
			client.EXPECT().Get(gomock.Any(), sysInfoOIDs()).Return(testSysInfoResponse([]gosnmp.VarBind{
				{Name: OidSysName, Value: gosnmp.OctetStringValue([]byte("printer"))},
				pdu,
			}), nil)

			si, err := GetSysInfo(context.Background(), client, gosnmp.Version2c, len(sysInfoOIDs()))
			require.NoError(t, err)
			require.NotNil(t, si)
			assert.Equal(t, tc.wantSysObject, si.SysObjectID)
			assert.Equal(t, "printer", si.Name)
			assert.Equal(t, SysInfoProbe{
				PDUCount:        2,
				SeenSysObjectID: true,
				SysObjectIDType: tc.wantType,
				SeenSysName:     true,
			}, si.Probe)
		})
	}
}

func TestGetSysInfoReturnsErrorForNilResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := snmpmock.NewMockSession(ctrl)
	client.EXPECT().Get(gomock.Any(), sysInfoOIDs()).Return(nil, nil)

	si, err := GetSysInfo(context.Background(), client, gosnmp.Version2c, len(sysInfoOIDs()))
	require.Error(t, err)
	assert.Nil(t, si)
	assert.Contains(t, err.Error(), "nil response")
}

func TestGetSysInfoAcceptsNoErrorWithNonzeroErrorIndex(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := snmpmock.NewMockSession(ctrl)
	client.EXPECT().Get(gomock.Any(), sysInfoOIDs()).Return(&gosnmp.Message{
		ErrorIndex: 1,
		VarBinds: []gosnmp.VarBind{
			{Name: OidSysObject, Value: gosnmp.ObjectIdentifierValue(".1.3.6.1.4.1.9.1.1166")},
		},
	}, nil)

	si, err := GetSysInfo(context.Background(), client, gosnmp.Version2c, len(sysInfoOIDs()))
	require.NoError(t, err)
	require.NotNil(t, si)
	assert.Equal(t, "1.3.6.1.4.1.9.1.1166", si.SysObjectID)
}

func TestGetSysInfoAcceptsExtraAndDuplicateResponseOIDs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := snmpmock.NewMockSession(ctrl)
	client.EXPECT().Get(gomock.Any(), sysInfoOIDs()).Return(testSysInfoResponse([]gosnmp.VarBind{
		{Name: "1.3.6.1.2.1.1.7.0", Value: gosnmp.IntegerValue(72)},
		{Name: OidSysObject, Value: gosnmp.ObjectIdentifierValue(".1.3.6.1.4.1.9.1.1")},
		{Name: "." + OidSysObject, Value: gosnmp.ObjectIdentifierValue(".1.3.6.1.4.1.9.1.1166")},
	}), nil)

	si, err := GetSysInfo(context.Background(), client, gosnmp.Version2c, len(sysInfoOIDs()))
	require.NoError(t, err)
	require.NotNil(t, si)
	assert.Equal(t, "1.3.6.1.4.1.9.1.1166", si.SysObjectID)
	assert.Equal(t, 3, si.Probe.PDUCount)
}

func TestGetSysInfoReturnsPacketErrors(t *testing.T) {
	tests := map[string]struct {
		version    gosnmp.Version
		status     gosnmp.ErrorStatus
		errorIndex uint8
		wantError  string
	}{
		"authorization error": {
			status:     gosnmp.AuthorizationError,
			errorIndex: 2,
			wantError:  "AuthorizationError",
		},
		"general error": {
			status:     gosnmp.GenErr,
			errorIndex: 4,
			wantError:  "GenErr",
		},
		"no such name with zero index": {
			status:    gosnmp.NoSuchName,
			wantError: "invalid error index 0",
		},
		"no such name with out-of-range index": {
			status:     gosnmp.NoSuchName,
			errorIndex: 6,
			wantError:  "invalid error index 6",
		},
		"no such name for configured v2c": {
			version:    gosnmp.Version2c,
			status:     gosnmp.NoSuchName,
			errorIndex: 2,
			wantError:  "unexpected response error NoSuchName for requested SNMP version 2c",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			client := snmpmock.NewMockSession(ctrl)
			client.EXPECT().Get(gomock.Any(), sysInfoOIDs()).Return(nil, &gosnmp.StatusError{Status: test.status, Index: test.errorIndex})

			si, err := GetSysInfo(context.Background(), client, test.version, len(sysInfoOIDs()))
			require.Error(t, err)
			assert.Nil(t, si)
			assert.Contains(t, err.Error(), test.wantError)
		})
	}
}

func TestGetSysInfoReturnsPartialResultForExceptionPDUs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := snmpmock.NewMockSession(ctrl)
	client.EXPECT().Get(gomock.Any(), sysInfoOIDs()).Return(testSysInfoResponse([]gosnmp.VarBind{
		{Name: OidSysDescr, Value: gosnmp.OctetStringValue([]byte("network device"))},
		{Name: OidSysObject, Value: gosnmp.NoSuchInstanceValue()},
		{Name: OidSysContact, Value: gosnmp.NoSuchObjectValue()},
		{Name: OidSysName, Value: gosnmp.OctetStringValue([]byte("router"))},
		{Name: OidSysLocation, Value: gosnmp.NullValue()},
	}), nil)

	si, err := GetSysInfo(context.Background(), client, gosnmp.Version2c, len(sysInfoOIDs()))
	require.NoError(t, err)
	require.NotNil(t, si)
	assert.Empty(t, si.SysObjectID)
	assert.Equal(t, SysInfoProbe{
		PDUCount:     5,
		SeenSysDescr: true,
		SeenSysName:  true,
	}, si.Probe)
}

func TestGetSysInfoRetriesIndexedNoSuchName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := snmpmock.NewMockSession(ctrl)
	gomock.InOrder(
		client.EXPECT().Get(gomock.Any(), []string{OidSysDescr, OidSysObject}).Return(testSysInfoResponse([]gosnmp.VarBind{
			{Name: OidSysDescr, Value: gosnmp.OctetStringValue([]byte("network device"))},
			{Name: OidSysObject, Value: gosnmp.ObjectIdentifierValue(".1.3.6.1.4.1.9.1.1166")},
		}), nil),
		client.EXPECT().Get(gomock.Any(), []string{OidSysContact, OidSysName}).Return(nil, &gosnmp.StatusError{Status: gosnmp.NoSuchName, Index: 1}),
		client.EXPECT().Get(gomock.Any(), []string{OidSysName}).Return(testSysInfoResponse([]gosnmp.VarBind{
			{Name: OidSysName, Value: gosnmp.OctetStringValue([]byte("router"))},
		}), nil),
		client.EXPECT().Get(gomock.Any(), []string{OidSysLocation}).Return(testSysInfoResponse([]gosnmp.VarBind{
			{Name: OidSysLocation, Value: gosnmp.OctetStringValue([]byte("datacenter"))},
		}), nil),
	)

	si, err := GetSysInfo(context.Background(), client, gosnmp.Version1, 2)
	require.NoError(t, err)
	require.NotNil(t, si)
	assert.Equal(t, "1.3.6.1.4.1.9.1.1166", si.SysObjectID)
	assert.False(t, si.Probe.SeenSysContact)
	assert.Equal(t, 4, si.Probe.PDUCount)
}

func TestGetSysInfoNoSuchNameRetriesAreBounded(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	requestCount := 0
	client := snmpmock.NewMockSession(ctrl)
	client.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, oids []string) (*gosnmp.Message, error) {
		requestCount++
		assert.Len(t, oids, 6-requestCount)
		return nil, &gosnmp.StatusError{Status: gosnmp.NoSuchName, Index: 1}
	}).Times(5)

	si, err := GetSysInfo(context.Background(), client, gosnmp.Version1, len(sysInfoOIDs()))
	require.NoError(t, err)
	require.NotNil(t, si)
	assert.Equal(t, 5, requestCount)
	assert.Equal(t, SysInfoProbe{}, si.Probe)
}

func TestSysInfoProbeIsNotSerialized(t *testing.T) {
	si := SysInfo{
		SysObjectID: "1.3.6.1.4.1.9.1.1166",
		Probe: SysInfoProbe{
			PDUCount:        2,
			SeenSysDescr:    true,
			SeenSysObjectID: true,
		},
	}

	got, err := json.Marshal(si)
	require.NoError(t, err)
	assert.NotContains(t, string(got), "Probe")
	assert.NotContains(t, string(got), "probe")
	assert.NotContains(t, string(got), OidSysDescr)
}

func testSysInfoResponse(pdus []gosnmp.VarBind) *gosnmp.Message {
	return &gosnmp.Message{VarBinds: pdus}
}
