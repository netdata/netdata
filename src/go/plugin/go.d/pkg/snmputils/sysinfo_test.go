// SPDX-License-Identifier: GPL-3.0-or-later

package snmputils

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSysInfoRecordsProbeDiagnostics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pdus := []gosnmp.SnmpPDU{
		{Name: "." + OidSysDescr, Type: gosnmp.OctetString, Value: []byte("network device")},
		{Name: OidSysObject, Type: gosnmp.ObjectIdentifier, Value: ".1.3.6.1.4.1.9.1.1166"},
		{Name: OidSysContact, Type: gosnmp.OctetString, Value: []byte("operations")},
		{Name: "." + OidSysName, Type: gosnmp.OctetString, Value: []byte("router")},
		{Name: OidSysLocation, Type: gosnmp.OctetString, Value: []byte("datacenter")},
	}

	client := snmpmock.NewMockHandler(ctrl)
	client.EXPECT().MaxOids().Return(len(sysInfoOIDs()))
	client.EXPECT().Version().Return(gosnmp.Version2c)
	client.EXPECT().Get(sysInfoOIDs()).Return(testSysInfoResponse(pdus), nil)

	si, err := GetSysInfo(client)
	require.NoError(t, err)
	require.NotNil(t, si)
	assert.Equal(t, "1.3.6.1.4.1.9.1.1166", si.SysObjectID)
	assert.Equal(t, SysInfoProbe{
		PDUCount:        len(pdus),
		SeenSysDescr:    true,
		SeenSysObjectID: true,
		SeenSysContact:  true,
		SeenSysName:     true,
		SeenSysLocation: true,
	}, si.Probe)
}

func TestGetSysInfoChunksRequestsByMaxOids(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := snmpmock.NewMockHandler(ctrl)
	gomock.InOrder(
		client.EXPECT().MaxOids().Return(2),
		client.EXPECT().Version().Return(gosnmp.Version2c),
		client.EXPECT().Get([]string{OidSysDescr, OidSysObject}).Return(testSysInfoResponse([]gosnmp.SnmpPDU{
			{Name: OidSysDescr, Type: gosnmp.OctetString, Value: []byte("network device")},
			{Name: OidSysObject, Type: gosnmp.ObjectIdentifier, Value: ".1.3.6.1.4.1.9.1.1166"},
		}), nil),
		client.EXPECT().Get([]string{OidSysContact, OidSysName}).Return(testSysInfoResponse([]gosnmp.SnmpPDU{
			{Name: OidSysContact, Type: gosnmp.OctetString, Value: []byte("operations")},
			{Name: OidSysName, Type: gosnmp.OctetString, Value: []byte("router")},
		}), nil),
		client.EXPECT().Get([]string{OidSysLocation}).Return(testSysInfoResponse([]gosnmp.SnmpPDU{
			{Name: OidSysLocation, Type: gosnmp.OctetString, Value: []byte("datacenter")},
		}), nil),
	)

	si, err := GetSysInfo(client)
	require.NoError(t, err)
	require.NotNil(t, si)
	assert.Equal(t, "1.3.6.1.4.1.9.1.1166", si.SysObjectID)
	assert.Equal(t, 5, si.Probe.PDUCount)
}

func TestGetSysInfoRejectsInvalidMaxOids(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := snmpmock.NewMockHandler(ctrl)
	client.EXPECT().MaxOids().Return(0)

	si, err := GetSysInfo(client)
	require.Error(t, err)
	assert.Nil(t, si)
	assert.Contains(t, err.Error(), "invalid maximum OIDs per request 0")
}

func TestGetSysInfoReturnsPartialProbeWithoutIdentityError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := snmpmock.NewMockHandler(ctrl)
	client.EXPECT().MaxOids().Return(len(sysInfoOIDs()))
	client.EXPECT().Version().Return(gosnmp.Version2c)
	client.EXPECT().Get(sysInfoOIDs()).Return(testSysInfoResponse([]gosnmp.SnmpPDU{
		{Name: OidSysDescr, Type: gosnmp.OctetString, Value: []byte("network device")},
		{Name: OidSysName, Type: gosnmp.OctetString, Value: []byte("router")},
	}), nil)

	si, err := GetSysInfo(client)
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

	client := snmpmock.NewMockHandler(ctrl)
	client.EXPECT().MaxOids().Return(len(sysInfoOIDs()))
	client.EXPECT().Version().Return(gosnmp.Version2c)
	client.EXPECT().Get(sysInfoOIDs()).Return(testSysInfoResponse(nil), nil)

	si, err := GetSysInfo(client)
	require.NoError(t, err)
	require.NotNil(t, si)
	assert.Empty(t, si.SysObjectID)
	assert.Equal(t, SysInfoProbe{}, si.Probe)
}

func TestGetSysInfoWrapsGetError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	getErr := errors.New("timeout")
	client := snmpmock.NewMockHandler(ctrl)
	client.EXPECT().MaxOids().Return(len(sysInfoOIDs()))
	client.EXPECT().Version().Return(gosnmp.Version2c)
	client.EXPECT().Get(sysInfoOIDs()).Return(nil, getErr)

	si, err := GetSysInfo(client)
	require.Error(t, err)
	assert.Nil(t, si)
	assert.ErrorIs(t, err, getErr)
	assert.Contains(t, err.Error(), "SNMP system scalars")
}

func TestGetSysInfoRejectsWrongTypedSysObjectWithoutEchoingValue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	const rawValue = "private device identifier"
	client := snmpmock.NewMockHandler(ctrl)
	client.EXPECT().MaxOids().Return(len(sysInfoOIDs()))
	client.EXPECT().Version().Return(gosnmp.Version2c)
	client.EXPECT().Get(sysInfoOIDs()).Return(testSysInfoResponse([]gosnmp.SnmpPDU{
		{Name: OidSysObject, Type: gosnmp.OctetString, Value: []byte(rawValue)},
	}), nil)

	si, err := GetSysInfo(client)
	require.Error(t, err)
	assert.Nil(t, si)
	assert.Contains(t, err.Error(), "expected ObjectIdentifier")
	assert.NotContains(t, err.Error(), rawValue)
}

func TestGetSysInfoReturnsErrorForNilResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := snmpmock.NewMockHandler(ctrl)
	client.EXPECT().MaxOids().Return(len(sysInfoOIDs()))
	client.EXPECT().Version().Return(gosnmp.Version2c)
	client.EXPECT().Get(sysInfoOIDs()).Return(nil, nil)

	si, err := GetSysInfo(client)
	require.Error(t, err)
	assert.Nil(t, si)
	assert.Contains(t, err.Error(), "nil response")
}

func TestGetSysInfoAcceptsNoErrorWithNonzeroErrorIndex(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := snmpmock.NewMockHandler(ctrl)
	client.EXPECT().MaxOids().Return(len(sysInfoOIDs()))
	client.EXPECT().Version().Return(gosnmp.Version2c)
	client.EXPECT().Get(sysInfoOIDs()).Return(&gosnmp.SnmpPacket{
		ErrorIndex: 1,
		Variables: []gosnmp.SnmpPDU{
			{Name: OidSysObject, Type: gosnmp.ObjectIdentifier, Value: ".1.3.6.1.4.1.9.1.1166"},
		},
	}, nil)

	si, err := GetSysInfo(client)
	require.NoError(t, err)
	require.NotNil(t, si)
	assert.Equal(t, "1.3.6.1.4.1.9.1.1166", si.SysObjectID)
}

func TestGetSysInfoAcceptsExtraAndDuplicateResponseOIDs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := snmpmock.NewMockHandler(ctrl)
	client.EXPECT().MaxOids().Return(len(sysInfoOIDs()))
	client.EXPECT().Version().Return(gosnmp.Version2c)
	client.EXPECT().Get(sysInfoOIDs()).Return(testSysInfoResponse([]gosnmp.SnmpPDU{
		{Name: "1.3.6.1.2.1.1.7.0", Type: gosnmp.Integer, Value: 72},
		{Name: OidSysObject, Type: gosnmp.ObjectIdentifier, Value: ".1.3.6.1.4.1.9.1.1"},
		{Name: "." + OidSysObject, Type: gosnmp.ObjectIdentifier, Value: ".1.3.6.1.4.1.9.1.1166"},
	}), nil)

	si, err := GetSysInfo(client)
	require.NoError(t, err)
	require.NotNil(t, si)
	assert.Equal(t, "1.3.6.1.4.1.9.1.1166", si.SysObjectID)
	assert.Equal(t, 3, si.Probe.PDUCount)
}

func TestGetSysInfoReturnsPacketErrors(t *testing.T) {
	tests := map[string]struct {
		version    gosnmp.SnmpVersion
		status     gosnmp.SNMPError
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

			client := snmpmock.NewMockHandler(ctrl)
			client.EXPECT().MaxOids().Return(len(sysInfoOIDs()))
			client.EXPECT().Version().Return(test.version)
			client.EXPECT().Get(sysInfoOIDs()).Return(&gosnmp.SnmpPacket{
				Error:      test.status,
				ErrorIndex: test.errorIndex,
			}, nil)

			si, err := GetSysInfo(client)
			require.Error(t, err)
			assert.Nil(t, si)
			assert.Contains(t, err.Error(), test.wantError)
		})
	}
}

func TestGetSysInfoReturnsPartialResultForExceptionPDUs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := snmpmock.NewMockHandler(ctrl)
	client.EXPECT().MaxOids().Return(len(sysInfoOIDs()))
	client.EXPECT().Version().Return(gosnmp.Version2c)
	client.EXPECT().Get(sysInfoOIDs()).Return(testSysInfoResponse([]gosnmp.SnmpPDU{
		{Name: OidSysDescr, Type: gosnmp.OctetString, Value: []byte("network device")},
		{Name: OidSysObject, Type: gosnmp.NoSuchInstance},
		{Name: OidSysContact, Type: gosnmp.NoSuchObject},
		{Name: OidSysName, Type: gosnmp.OctetString, Value: []byte("router")},
		{Name: OidSysLocation, Type: gosnmp.Null},
	}), nil)

	si, err := GetSysInfo(client)
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

	client := snmpmock.NewMockHandler(ctrl)
	gomock.InOrder(
		client.EXPECT().MaxOids().Return(2),
		client.EXPECT().Version().Return(gosnmp.Version1),
		client.EXPECT().Get([]string{OidSysDescr, OidSysObject}).Return(testSysInfoResponse([]gosnmp.SnmpPDU{
			{Name: OidSysDescr, Type: gosnmp.OctetString, Value: []byte("network device")},
			{Name: OidSysObject, Type: gosnmp.ObjectIdentifier, Value: ".1.3.6.1.4.1.9.1.1166"},
		}), nil),
		client.EXPECT().Get([]string{OidSysContact, OidSysName}).Return(&gosnmp.SnmpPacket{
			Error:      gosnmp.NoSuchName,
			ErrorIndex: 1,
		}, nil),
		client.EXPECT().Get([]string{OidSysName}).Return(testSysInfoResponse([]gosnmp.SnmpPDU{
			{Name: OidSysName, Type: gosnmp.OctetString, Value: []byte("router")},
		}), nil),
		client.EXPECT().Get([]string{OidSysLocation}).Return(testSysInfoResponse([]gosnmp.SnmpPDU{
			{Name: OidSysLocation, Type: gosnmp.OctetString, Value: []byte("datacenter")},
		}), nil),
	)

	si, err := GetSysInfo(client)
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
	client := snmpmock.NewMockHandler(ctrl)
	client.EXPECT().MaxOids().Return(len(sysInfoOIDs()))
	client.EXPECT().Version().Return(gosnmp.Version1)
	client.EXPECT().Get(gomock.Any()).DoAndReturn(func(oids []string) (*gosnmp.SnmpPacket, error) {
		requestCount++
		assert.Len(t, oids, 6-requestCount)
		return &gosnmp.SnmpPacket{
			Error: gosnmp.NoSuchName, ErrorIndex: 1,
		}, nil
	}).Times(5)

	si, err := GetSysInfo(client)
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

func testSysInfoResponse(pdus []gosnmp.SnmpPDU) *gosnmp.SnmpPacket {
	return &gosnmp.SnmpPacket{Variables: pdus}
}
