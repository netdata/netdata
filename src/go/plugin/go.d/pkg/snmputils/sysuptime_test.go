// SPDX-License-Identifier: GPL-3.0-or-later

package snmputils

import (
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSysUptime(t *testing.T) {
	tests := map[string]struct {
		pdus     []gosnmp.SnmpPDU
		getErr   error
		expected int64
		wantErr  bool
	}{
		"prefers_snmp_engine_time_seconds": {
			pdus: []gosnmp.SnmpPDU{
				{Name: OidSnmpEngineTime, Type: gosnmp.Integer, Value: 1234},
				{Name: OidHrSystemUptime, Type: gosnmp.TimeTicks, Value: uint32(999900)},
				{Name: OidSysUpTime, Type: gosnmp.TimeTicks, Value: uint32(888800)},
			},
			expected: 1234,
		},
		"falls_back_to_host_resources_timeticks": {
			pdus: []gosnmp.SnmpPDU{
				{Name: OidSnmpEngineTime, Type: gosnmp.NoSuchObject, Value: nil},
				{Name: "." + OidHrSystemUptime, Type: gosnmp.TimeTicks, Value: uint32(123456)},
				{Name: OidSysUpTime, Type: gosnmp.TimeTicks, Value: uint32(888800)},
			},
			expected: 1234,
		},
		"falls_back_to_mib2_sysuptime_timeticks": {
			pdus: []gosnmp.SnmpPDU{
				{Name: OidSnmpEngineTime, Type: gosnmp.NoSuchInstance, Value: nil},
				{Name: OidHrSystemUptime, Type: gosnmp.NoSuchObject, Value: nil},
				{Name: OidSysUpTime, Type: gosnmp.TimeTicks, Value: uint32(987654)},
			},
			expected: 9876,
		},
		"returns_zero_when_no_source_has_data": {
			pdus: []gosnmp.SnmpPDU{
				{Name: OidSnmpEngineTime, Type: gosnmp.NoSuchObject, Value: nil},
				{Name: OidHrSystemUptime, Type: gosnmp.NoSuchObject, Value: nil},
				{Name: OidSysUpTime, Type: gosnmp.NoSuchObject, Value: nil},
			},
			expected: 0,
		},
		"returns_get_error": {
			getErr:  errors.New("timeout"),
			wantErr: true,
		},
		"returns_conversion_error_when_only_available_source_is_invalid": {
			pdus: []gosnmp.SnmpPDU{
				{Name: OidSnmpEngineTime, Type: gosnmp.OctetString, Value: []byte("bad")},
				{Name: OidHrSystemUptime, Type: gosnmp.NoSuchObject, Value: nil},
				{Name: OidSysUpTime, Type: gosnmp.NoSuchObject, Value: nil},
			},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			client := snmpmock.NewMockHandler(ctrl)
			client.EXPECT().Get(gomock.InAnyOrder(sysUptimeOIDs())).Return(&gosnmp.SnmpPacket{Variables: tc.pdus}, tc.getErr)

			actual, err := GetSysUptime(client)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.expected, actual)
		})
	}
}
