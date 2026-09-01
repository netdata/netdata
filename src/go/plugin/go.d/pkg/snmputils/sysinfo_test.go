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
		{Name: "1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(123)},
		{Name: "." + OidSysName, Type: gosnmp.OctetString, Value: []byte("router")},
		{Name: OidSysLocation, Type: gosnmp.OctetString, Value: []byte("datacenter")},
	}

	client := snmpmock.NewMockHandler(ctrl)
	client.EXPECT().WalkAll(RootOidMibSystem).Return(pdus, nil)

	si, err := GetSysInfo(client)
	require.NoError(t, err)
	require.NotNil(t, si)
	assert.Equal(t, "1.3.6.1.4.1.9.1.1166", si.SysObjectID)
	assert.Equal(t, SysInfoProbe{
		PDUCount:        len(pdus),
		FirstOID:        OidSysDescr,
		LastOID:         OidSysLocation,
		SeenSysDescr:    true,
		SeenSysObjectID: true,
		SeenSysContact:  true,
		SeenSysName:     true,
		SeenSysLocation: true,
	}, si.Probe)
}

func TestGetSysInfoReturnsPartialProbeWithoutIdentityError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := snmpmock.NewMockHandler(ctrl)
	client.EXPECT().WalkAll(RootOidMibSystem).Return([]gosnmp.SnmpPDU{
		{Name: OidSysDescr, Type: gosnmp.OctetString, Value: []byte("network device")},
		{Name: OidSysName, Type: gosnmp.OctetString, Value: []byte("router")},
	}, nil)

	si, err := GetSysInfo(client)
	require.NoError(t, err)
	require.NotNil(t, si)
	assert.Empty(t, si.SysObjectID)
	assert.Equal(t, SysInfoProbe{
		PDUCount:     2,
		FirstOID:     OidSysDescr,
		LastOID:      OidSysName,
		SeenSysDescr: true,
		SeenSysName:  true,
	}, si.Probe)
}

func TestGetSysInfoReturnsEmptyProbeWithoutIdentityError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := snmpmock.NewMockHandler(ctrl)
	client.EXPECT().WalkAll(RootOidMibSystem).Return(nil, nil)

	si, err := GetSysInfo(client)
	require.NoError(t, err)
	require.NotNil(t, si)
	assert.Empty(t, si.SysObjectID)
	assert.Equal(t, SysInfoProbe{}, si.Probe)
}

func TestGetSysInfoWrapsWalkErrorWithSystemRoot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	walkErr := errors.New("timeout")
	client := snmpmock.NewMockHandler(ctrl)
	client.EXPECT().WalkAll(RootOidMibSystem).Return(nil, walkErr)

	si, err := GetSysInfo(client)
	require.Error(t, err)
	assert.Nil(t, si)
	assert.ErrorIs(t, err, walkErr)
	assert.Contains(t, err.Error(), RootOidMibSystem)
}

func TestGetSysInfoRejectsWrongTypedSysObjectWithoutEchoingValue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	const rawValue = "private device identifier"
	client := snmpmock.NewMockHandler(ctrl)
	client.EXPECT().WalkAll(RootOidMibSystem).Return([]gosnmp.SnmpPDU{
		{Name: OidSysObject, Type: gosnmp.OctetString, Value: []byte(rawValue)},
	}, nil)

	si, err := GetSysInfo(client)
	require.Error(t, err)
	assert.Nil(t, si)
	assert.Contains(t, err.Error(), "expected ObjectIdentifier")
	assert.NotContains(t, err.Error(), rawValue)
}

func TestSysInfoProbeIsNotSerialized(t *testing.T) {
	si := SysInfo{
		SysObjectID: "1.3.6.1.4.1.9.1.1166",
		Probe: SysInfoProbe{
			PDUCount:        2,
			FirstOID:        OidSysDescr,
			LastOID:         OidSysObject,
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
