// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiskInventory(t *testing.T) {
	singleDrive := fixtureRows(map[string]any{"DeviceID": "c:"}).rows
	twoDrives := fixtureRows(
		map[string]any{"DeviceID": "C:"},
		map[string]any{"DeviceID": "D:"},
	).rows
	diskExtent := func(path string, start, end any) map[string]any {
		return map[string]any{"Dependent": path, "StartingAddress": start, "EndingAddress": end}
	}
	const (
		cPath = "\\\\WINDOWS\\root\\cimv2:Win32_LogicalDisk.DeviceID=\"C:\""
		dPath = "Win32_LogicalDisk.DeviceID=\"D:\""
		ePath = "win32_logicaldisk.deviceid=\"e:\""
	)
	cExtent := diskExtent(cPath, "4096", "8191")
	dExtent := diskExtent(dPath, uint64(0), uint64(16383))
	queryErr := errors.New("association unavailable")
	tests := map[string]struct {
		drives []map[string]any
		result queryResult
		want   string
		err    error
	}{
		"inclusive extents for fixed letters only": {
			drives: twoDrives,
			result: fixtureRows(dExtent, diskExtent(ePath, nil, nil), cExtent),
			want:   "20480",
		},
		"complete sum at uint64 limit": {
			drives: singleDrive,
			result: fixtureRows(diskExtent(cPath, uint64(1), uint64(math.MaxUint64))),
			want:   "18446744073709551615",
		},
		"missing drive mapping": {
			drives: twoDrives,
			result: fixtureRows(cExtent),
		},
		"multiple extents for one drive": {
			drives: singleDrive,
			result: fixtureRows(cExtent, diskExtent(cPath, "8192", "12287")),
		},
		"missing fixed drive letter": {
			drives: fixtureRows(map[string]any{"DeviceID": "C:"}, map[string]any{}).rows,
		},
		"invalid association reference": {
			drives: singleDrive,
			result: fixtureRows(cExtent, diskExtent("Different_Class.DeviceID=\"D:\"", uint64(0), uint64(1))),
		},
		"invalid extent": {
			drives: singleDrive,
			result: fixtureRows(diskExtent(cPath, "2", "1")),
		},
		"missing extent bound": {
			drives: singleDrive,
			result: fixtureRows(diskExtent(cPath, nil, "1")),
		},
		"extent length overflow": {
			drives: singleDrive,
			result: fixtureRows(diskExtent(cPath, uint64(0), uint64(math.MaxUint64))),
		},
		"aggregate overflow": {
			drives: twoDrives,
			result: fixtureRows(diskExtent(cPath, uint64(1), uint64(math.MaxUint64)), dExtent),
		},
		"query error": {
			drives: singleDrive,
			result: queryResult{err: queryErr},
			err:    queryErr,
		},
		"no fixed drives": {},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			session := &fakeSession{results: map[string]queryResult{queryDiskExtents: tt.result}}
			got, err := diskInventory(t.Context(), session, tt.drives)
			require.ErrorIs(t, err, tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNetworkInventory(t *testing.T) {
	route := map[string]any{"InterfaceIndex": uint32(math.MaxUint32)}
	adapter := map[string]any{"InterfaceIndex": "4294967295", "NetConnectionID": " Ethernet 2 "}
	ipConfig := func(addresses ...string) map[string]any {
		return map[string]any{"InterfaceIndex": uint32(math.MaxUint32), "IPAddress": addresses}
	}
	adapters := fixtureRows(adapter)
	config := fixtureRows(ipConfig("fe80::1", "192.0.2.7", "192.0.2.7"))
	queryErr := errors.New("configuration unavailable")
	tests := map[string]struct {
		routes   queryResult
		adapters queryResult
		configs  queryResult
		want     [2]string
		err      error
	}{
		"distinct default routes on the same interface": {
			routes:   fixtureRows(route, map[string]any{"InterfaceIndex": "4294967295"}),
			adapters: adapters,
			configs:  config,
			want:     [2]string{"Ethernet 2", "192.0.2.7"},
		},
		"different default interfaces": {
			routes: fixtureRows(route, map[string]any{"InterfaceIndex": uint32(12)}),
		},
		"missing route index": {
			routes: fixtureRows(route, map[string]any{}),
		},
		"invalid route index": {
			routes: fixtureRows(map[string]any{"InterfaceIndex": uint64(math.MaxUint32) + 1}),
		},
		"no default route": {},
		"wrong adapter": {
			routes:   fixtureRows(route),
			adapters: fixtureRows(map[string]any{"InterfaceIndex": uint32(12), "NetConnectionID": "Ethernet"}),
			configs:  config,
		},
		"ambiguous adapter": {
			routes:   fixtureRows(route),
			adapters: fixtureRows(adapter, adapter),
			configs:  config,
		},
		"multiple usable IPv4 addresses": {
			routes:   fixtureRows(route),
			adapters: adapters,
			configs:  fixtureRows(ipConfig("192.0.2.7", "192.0.2.8")),
		},
		"no usable IPv4 address": {
			routes:   fixtureRows(route),
			adapters: adapters,
			configs:  fixtureRows(ipConfig("fe80::1", "127.0.0.1", "169.254.1.1", "0.0.0.0", "224.0.0.1", "255.255.255.255")),
		},
		"malformed address": {
			routes:   fixtureRows(route),
			adapters: adapters,
			configs:  fixtureRows(ipConfig("192.0.2.7", "malformed")),
		},
		"query error": {
			routes:   fixtureRows(route),
			adapters: adapters,
			configs:  queryResult{err: queryErr},
			err:      queryErr,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			session := &fakeSession{results: map[string]queryResult{
				queryDefaultRoutes:              tt.routes,
				queryAdapterName + "4294967295": tt.adapters,
				queryAdapterIPs + "4294967295":  tt.configs,
			}}
			name, ip, err := networkInventory(t.Context(), session)
			require.ErrorIs(t, err, tt.err)
			assert.Equal(t, tt.want, [2]string{name, ip})
		})
	}
}
