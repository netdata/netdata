// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"testing"
	"time"

	"github.com/bougou/go-ipmi/pkg/command/storage"
	"github.com/bougou/go-ipmi/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReader_InventoryCache(t *testing.T) {
	bmc := newFakeBMC()
	r := newTestReader(bmc)
	now := testNow
	r.now = func() time.Time { return now }

	for range 2 {
		_, err := r.Collect(t.Context(), false)
		require.NoError(t, err)
	}
	assert.Len(t, bmc.sdrRequests(), 2, "an unchanged repository is discovered once")

	// The sentinel timestamps never change; the maximum age still picks up a renamed sensor.
	bmc.records[0] = fullRecord(1, 10, "New Name")
	now = now.Add(inventoryMaxAge)
	got, err := r.Collect(t.Context(), false)
	require.NoError(t, err)
	assert.Equal(t, "New Name", got.Sensors[0].Name)
	assert.Len(t, bmc.sdrRequests(), 4)

	// A repository change is discovered immediately.
	bmc.additionTime = 123
	bmc.records = bmc.records[:1]
	got, err = r.Collect(t.Context(), false)
	require.NoError(t, err)
	assert.Len(t, got.Sensors, 1)
	assert.Len(t, bmc.sdrRequests(), 5)
}

func TestReader_InventoryEmpty(t *testing.T) {
	bmc := newFakeBMC()
	bmc.records = nil

	got, err := newTestReader(bmc).Collect(t.Context(), true)
	require.NoError(t, err)
	assert.Equal(t, &Snapshot{
		Sensors:     []Sensor{},
		SELEntries:  new(fakeSELEntries),
		CollectedAt: testNow,
	}, got)
}

func TestReader_InventoryRejected(t *testing.T) {
	tests := map[string]struct {
		prepare func(*fakeBMC)
	}{
		"repository changed during discovery": {
			prepare: func(b *fakeBMC) {
				b.onExchange = func(req types.Request) error {
					if _, ok := req.(*storage.GetSDRRequest); ok {
						b.additionTime++
					}
					return nil
				}
			},
		},
		"record shorter than its header": {
			prepare: func(b *fakeBMC) { b.records[0] = b.records[0][:types.SDRRecordHeaderSize] },
		},
		"cyclic record chain": {
			prepare: func(b *fakeBMC) {
				b.onExchange = func(req types.Request) error {
					if q, ok := req.(*storage.GetSDRRequest); ok {
						q.RecordID = firstRecordID
					}
					return nil
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			bmc := newFakeBMC()
			tc.prepare(bmc)

			got, err := newTestReader(bmc).Collect(t.Context(), false)
			require.Error(t, err)
			assert.Nil(t, got, "no snapshot from an inconsistent inventory")
		})
	}
}

func TestReader_PartialRecordReads(t *testing.T) {
	bmc := newFakeBMC()
	bmc.refuseWholeReads = true

	got, err := newTestReader(bmc).Collect(t.Context(), false)
	require.NoError(t, err)
	assert.Equal(t, &Snapshot{
		Sensors:     []Sensor{wantCPUTemp(StateNominal, new(25.0)), wantCPUPresence(StateNominal)},
		CollectedAt: testNow,
	}, got)
}

func TestReader_PartialRecordReadsMaximumRecord(t *testing.T) {
	// An OEM record with the largest body: header and body reach offset 259.
	record := make([]byte, types.SDRRecordHeaderSize+255)
	record[2] = 0x51 // SDR version
	record[3] = 0xc0 // OEM record type
	record[recordLengthOffset] = 255
	bmc := newFakeBMC()
	bmc.records = [][]byte{record}
	bmc.refuseWholeReads = true

	got, err := newTestReader(bmc).Collect(t.Context(), false)
	require.NoError(t, err)
	assert.Empty(t, got.Sensors, "an OEM record is not a sensor")

	requests := bmc.sdrRequests()
	require.NotEmpty(t, requests)
	assert.Equal(t, &storage.GetSDRRequest{
		ReservationID: 1,
		RecordID:      firstRecordID,
		ReadOffset:    maxReadOffset,
		ReadBytes:     5,
	}, requests[len(requests)-1], "the last chunk starts at the last addressable offset")
}
