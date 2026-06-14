// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package nvme

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWindowsNvmeHealthLogToSmartLog(t *testing.T) {
	logPage := make([]byte, nvmeHealthLogSize)
	logPage[0] = 0x02
	binary.LittleEndian.PutUint16(logPage[1:3], 303)
	logPage[3] = 98
	logPage[4] = 10
	logPage[5] = 7
	putUint128(logPage[32:48], 11)
	putUint128(logPage[48:64], 12)
	putUint128(logPage[64:80], 13)
	putUint128(logPage[80:96], 14)
	putUint128(logPage[96:112], 15)
	putUint128(logPage[112:128], 16)
	putUint128(logPage[128:144], 17)
	putUint128(logPage[144:160], 18)
	putUint128(logPage[160:176], 19)
	putUint128(logPage[176:192], 20)
	binary.LittleEndian.PutUint32(logPage[192:196], 21)
	binary.LittleEndian.PutUint32(logPage[196:200], 22)
	binary.LittleEndian.PutUint32(logPage[216:220], 23)
	binary.LittleEndian.PutUint32(logPage[220:224], 24)
	binary.LittleEndian.PutUint32(logPage[224:228], 25)
	binary.LittleEndian.PutUint32(logPage[228:232], 26)

	got, err := windowsNvmeHealthLogToSmartLog(logPage)

	require.NoError(t, err)
	assert.Equal(t, nvmeNumber("2"), got.CriticalWarningValue)
	assert.Equal(t, nvmeNumber("303"), got.Temperature)
	assert.Equal(t, nvmeNumber("98"), got.AvailSpare)
	assert.Equal(t, nvmeNumber("10"), got.SpareThresh)
	assert.Equal(t, nvmeNumber("7"), got.PercentUsed)
	assert.Equal(t, nvmeNumber("11"), got.DataUnitsRead)
	assert.Equal(t, nvmeNumber("12"), got.DataUnitsWritten)
	assert.Equal(t, nvmeNumber("13"), got.HostReadCommands)
	assert.Equal(t, nvmeNumber("14"), got.HostWriteCommands)
	assert.Equal(t, nvmeNumber("15"), got.ControllerBusyTime)
	assert.Equal(t, nvmeNumber("16"), got.PowerCycles)
	assert.Equal(t, nvmeNumber("17"), got.PowerOnHours)
	assert.Equal(t, nvmeNumber("18"), got.UnsafeShutdowns)
	assert.Equal(t, nvmeNumber("19"), got.MediaErrors)
	assert.Equal(t, nvmeNumber("20"), got.NumErrLogEntries)
	assert.Equal(t, nvmeNumber("21"), got.WarningTempTime)
	assert.Equal(t, nvmeNumber("22"), got.CriticalCompTime)
	assert.Equal(t, nvmeNumber("23"), got.ThmTemp1TransCount)
	assert.Equal(t, nvmeNumber("24"), got.ThmTemp2TransCount)
	assert.Equal(t, nvmeNumber("25"), got.ThmTemp1TotalTime)
	assert.Equal(t, nvmeNumber("26"), got.ThmTemp2TotalTime)
}

func TestNewWindowsStoragePropertyQuery(t *testing.T) {
	query := newWindowsStoragePropertyQuery(storageDeviceProperty)

	require.Len(t, query, storagePropertyQuerySize)
	assert.Equal(t, uint32(storageDeviceProperty), binary.LittleEndian.Uint32(query[0:4]))
	assert.Equal(t, uint32(propertyStandardQuery), binary.LittleEndian.Uint32(query[4:8]))
	assert.Equal(t, make([]byte, storagePropertyQuerySize-storagePropertyQueryHeaderSize), query[storagePropertyQueryHeaderSize:])
}

func TestLittleEndianUint128ToInt64(t *testing.T) {
	tests := map[string]struct {
		prepare func([]byte)
		want    int64
	}{
		"lower 64 bits": {
			prepare: func(b []byte) { binary.LittleEndian.PutUint64(b[:8], 42) },
			want:    42,
		},
		"upper bits clamp": {
			prepare: func(b []byte) { b[8] = 1 },
			want:    math.MaxInt64,
		},
		"lower overflow clamps": {
			prepare: func(b []byte) { binary.LittleEndian.PutUint64(b[:8], math.MaxUint64) },
			want:    math.MaxInt64,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			b := make([]byte, 16)
			test.prepare(b)
			assert.Equal(t, test.want, littleEndianUint128ToInt64(b))
		})
	}
}

func TestLittleEndianUint128ToInt64Max(t *testing.T) {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint64(b[:8], 101)
	assert.Equal(t, int64(100), littleEndianUint128ToInt64Max(b, 100))

	binary.LittleEndian.PutUint64(b[:8], 99)
	b[8] = 1
	assert.Equal(t, int64(100), littleEndianUint128ToInt64Max(b, 100))
}

func putUint128(b []byte, v uint64) {
	binary.LittleEndian.PutUint64(b[:8], v)
}
