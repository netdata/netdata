// SPDX-License-Identifier: GPL-3.0-or-later

package snmputils

import (
	"context"
	"errors"
	"testing"

	"github.com/netdata/gosnmp"
	snmpmock "github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils/snmpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGetSysUptime(t *testing.T) {
	tests := map[string]struct {
		pdus     []gosnmp.VarBind
		getErr   error
		expected int64
		wantErr  bool
	}{
		"prefers_snmp_engine_time_seconds": {
			pdus: []gosnmp.VarBind{
				{Name: OidSnmpEngineTime, Value: gosnmp.IntegerValue(1234)},
				{Name: OidHrSystemUptime, Value: gosnmp.TimeTicksValue(999900)},
				{Name: OidSysUpTime, Value: gosnmp.TimeTicksValue(888800)},
			},
			expected: 1234,
		},
		"falls_back_to_host_resources_timeticks": {
			pdus: []gosnmp.VarBind{
				{Name: OidSnmpEngineTime, Value: gosnmp.NoSuchObjectValue()},
				{Name: "." + OidHrSystemUptime, Value: gosnmp.TimeTicksValue(123456)},
				{Name: OidSysUpTime, Value: gosnmp.TimeTicksValue(888800)},
			},
			expected: 1234,
		},
		"falls_back_to_mib2_sysuptime_timeticks": {
			pdus: []gosnmp.VarBind{
				{Name: OidSnmpEngineTime, Value: gosnmp.NoSuchInstanceValue()},
				{Name: OidHrSystemUptime, Value: gosnmp.NoSuchObjectValue()},
				{Name: OidSysUpTime, Value: gosnmp.TimeTicksValue(987654)},
			},
			expected: 9876,
		},
		"returns_zero_when_no_source_has_data": {
			pdus: []gosnmp.VarBind{
				{Name: OidSnmpEngineTime, Value: gosnmp.NoSuchObjectValue()},
				{Name: OidHrSystemUptime, Value: gosnmp.NoSuchObjectValue()},
				{Name: OidSysUpTime, Value: gosnmp.NoSuchObjectValue()},
			},
			expected: 0,
		},
		"returns_get_error": {
			getErr:  errors.New("timeout"),
			wantErr: true,
		},
		"returns_conversion_error_when_only_available_source_is_invalid": {
			pdus: []gosnmp.VarBind{
				{Name: OidSnmpEngineTime, Value: gosnmp.OctetStringValue([]byte("bad"))},
				{Name: OidHrSystemUptime, Value: gosnmp.NoSuchObjectValue()},
				{Name: OidSysUpTime, Value: gosnmp.NoSuchObjectValue()},
			},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			client := snmpmock.NewMockSession(ctrl)
			client.EXPECT().Get(gomock.Any(), gomock.InAnyOrder(sysUptimeOIDs())).Return(&gosnmp.Message{VarBinds: tc.pdus}, tc.getErr)

			actual, err := GetSysUptime(context.Background(), client)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.expected, actual)
		})
	}
}
