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
	client.EXPECT().Get(sysInfoOIDs()).Return(testSysInfoResponse(gosnmp.Version2c, pdus), nil)

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
		client.EXPECT().Get([]string{OidSysDescr, OidSysObject}).Return(testSysInfoResponse(gosnmp.Version2c, []gosnmp.SnmpPDU{
			{Name: OidSysDescr, Type: gosnmp.OctetString, Value: []byte("network device")},
			{Name: OidSysObject, Type: gosnmp.ObjectIdentifier, Value: ".1.3.6.1.4.1.9.1.1166"},
		}), nil),
		client.EXPECT().Get([]string{OidSysContact, OidSysName}).Return(testSysInfoResponse(gosnmp.Version2c, []gosnmp.SnmpPDU{
			{Name: OidSysContact, Type: gosnmp.OctetString, Value: []byte("operations")},
			{Name: OidSysName, Type: gosnmp.OctetString, Value: []byte("router")},
		}), nil),
		client.EXPECT().Get([]string{OidSysLocation}).Return(testSysInfoResponse(gosnmp.Version2c, []gosnmp.SnmpPDU{
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
	client.EXPECT().Get(sysInfoOIDs()).Return(testSysInfoResponse(gosnmp.Version2c, []gosnmp.SnmpPDU{
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
	client.EXPECT().Get(sysInfoOIDs()).Return(testSysInfoResponse(gosnmp.Version2c, nil), nil)

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
	client.EXPECT().Get(sysInfoOIDs()).Return(testSysInfoResponse(gosnmp.Version2c, []gosnmp.SnmpPDU{
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

func TestGetSysInfoRejectsUnexpectedResponseEnvelope(t *testing.T) {
	tests := map[string]struct {
		packet    *gosnmp.SnmpPacket
		wantError string
	}{
		"non-response PDU": {
			packet: &gosnmp.SnmpPacket{
				Version: gosnmp.Version2c,
				PDUType: gosnmp.GetRequest,
				Variables: []gosnmp.SnmpPDU{
					{Name: OidSysDescr, Type: gosnmp.OctetString, Value: []byte("network device")},
				},
			},
			wantError: "unexpected response PDU type GetRequest",
		},
		"mismatched version": {
			packet: &gosnmp.SnmpPacket{
				Version: gosnmp.Version1,
				PDUType: gosnmp.GetResponse,
				Variables: []gosnmp.SnmpPDU{
					{Name: OidSysDescr, Type: gosnmp.OctetString, Value: []byte("network device")},
				},
			},
			wantError: "response SNMP version 1 does not match requested version 2c",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			client := snmpmock.NewMockHandler(ctrl)
			client.EXPECT().MaxOids().Return(len(sysInfoOIDs()))
			client.EXPECT().Version().Return(gosnmp.Version2c)
			client.EXPECT().Get(sysInfoOIDs()).Return(test.packet, nil)

			si, err := GetSysInfo(client)
			require.Error(t, err)
			assert.Nil(t, si)
			assert.Contains(t, err.Error(), test.wantError)
		})
	}
}

func TestGetSysInfoRejectsUnexpectedResponseVarbinds(t *testing.T) {
	tests := map[string]struct {
		maxOids   int
		request   []string
		variables []gosnmp.SnmpPDU
		wantError string
	}{
		"unrequested OID": {
			maxOids: len(sysInfoOIDs()),
			request: sysInfoOIDs(),
			variables: []gosnmp.SnmpPDU{
				{Name: "1.3.6.1.2.1.1.7.0", Type: gosnmp.Integer, Value: 72},
			},
			wantError: "unrequested OID",
		},
		"duplicate OID": {
			maxOids: len(sysInfoOIDs()),
			request: sysInfoOIDs(),
			variables: []gosnmp.SnmpPDU{
				{Name: OidSysDescr, Type: gosnmp.OctetString, Value: []byte("network device")},
				{Name: "." + OidSysDescr, Type: gosnmp.OctetString, Value: []byte("duplicate")},
			},
			wantError: "duplicate OID",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			client := snmpmock.NewMockHandler(ctrl)
			client.EXPECT().MaxOids().Return(test.maxOids)
			client.EXPECT().Version().Return(gosnmp.Version2c)
			client.EXPECT().Get(test.request).Return(&gosnmp.SnmpPacket{
				Version:   gosnmp.Version2c,
				PDUType:   gosnmp.GetResponse,
				Variables: test.variables,
			}, nil)

			si, err := GetSysInfo(client)
			require.Error(t, err)
			assert.Nil(t, si)
			assert.Contains(t, err.Error(), test.wantError)
		})
	}
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
		"no such name from v2c response": {
			version:    gosnmp.Version2c,
			status:     gosnmp.NoSuchName,
			errorIndex: 2,
			wantError:  "unexpected response error NoSuchName for SNMP version 2c",
		},
		"no error with nonzero index": {
			status:     gosnmp.NoError,
			errorIndex: 1,
			wantError:  "response error NoError with nonzero error index 1",
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
				Version:    test.version,
				PDUType:    gosnmp.GetResponse,
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
	client.EXPECT().Get(sysInfoOIDs()).Return(testSysInfoResponse(gosnmp.Version2c, []gosnmp.SnmpPDU{
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
		client.EXPECT().Get([]string{OidSysDescr, OidSysObject}).Return(testSysInfoResponse(gosnmp.Version1, []gosnmp.SnmpPDU{
			{Name: OidSysDescr, Type: gosnmp.OctetString, Value: []byte("network device")},
			{Name: OidSysObject, Type: gosnmp.ObjectIdentifier, Value: ".1.3.6.1.4.1.9.1.1166"},
		}), nil),
		client.EXPECT().Get([]string{OidSysContact, OidSysName}).Return(&gosnmp.SnmpPacket{
			Version:    gosnmp.Version1,
			PDUType:    gosnmp.GetResponse,
			Error:      gosnmp.NoSuchName,
			ErrorIndex: 1,
		}, nil),
		client.EXPECT().Get([]string{OidSysName}).Return(testSysInfoResponse(gosnmp.Version1, []gosnmp.SnmpPDU{
			{Name: OidSysName, Type: gosnmp.OctetString, Value: []byte("router")},
		}), nil),
		client.EXPECT().Get([]string{OidSysLocation}).Return(testSysInfoResponse(gosnmp.Version1, []gosnmp.SnmpPDU{
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
			Version: gosnmp.Version1, PDUType: gosnmp.GetResponse, Error: gosnmp.NoSuchName, ErrorIndex: 1,
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

func testSysInfoResponse(version gosnmp.SnmpVersion, pdus []gosnmp.SnmpPDU) *gosnmp.SnmpPacket {
	return &gosnmp.SnmpPacket{Version: version, PDUType: gosnmp.GetResponse, Variables: pdus}
}
