// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiskCapacity(t *testing.T) {
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
			result: queryResult{
				err: queryErr,
			},
			err: queryErr,
		},
		"no fixed drives": {},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			session := &fakeSession{
				results: map[string]queryResult{queryDiskExtents: tt.result},
			}
			got, err := diskCapacity(t.Context(), session, tt.drives)
			require.ErrorIs(t, err, tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}
