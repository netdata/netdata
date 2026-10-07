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
	fake := newFakeBMC()
	r := newTestReader(fake)
	now := testNow
	r.now = func() time.Time { return now }

	for range 2 {
		_, err := r.Collect(t.Context(), false)
		require.NoError(t, err)
	}
	assert.Len(t, fake.sdrRequests(), 2, "an unchanged repository is discovered once")

	// The sentinel timestamps never change; the maximum age still picks up a renamed sensor.
	fake.records[0] = fullRecord(1, 10, "New Name")
	now = now.Add(inventoryMaxAge)
	got, err := r.Collect(t.Context(), false)
	require.NoError(t, err)
	assert.Equal(t, "New Name", got.Sensors[0].Name)
	assert.Len(t, fake.sdrRequests(), 4)

	// A repository change is discovered immediately.
	fake.additionTime = 123
	fake.records = fake.records[:1]
	got, err = r.Collect(t.Context(), false)
	require.NoError(t, err)
	assert.Len(t, got.Sensors, 1)
	assert.Len(t, fake.sdrRequests(), 5)
}

func TestReader_InventoryEmpty(t *testing.T) {
	fake := newFakeBMC()
	fake.records = nil

	got, err := newTestReader(fake).Collect(t.Context(), true)
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
		wantErr string
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
			wantErr: "SDR repository changed during discovery",
		},
		"record shorter than its header": {
			prepare: func(b *fakeBMC) { b.records[0] = b.records[0][:types.SDRRecordHeaderSize] },
			wantErr: "length does not match header",
		},
		"chain continues past the record count": {
			prepare: func(fake *fakeBMC) { fake.recordCount = 1 },
			wantErr: "continues past the repository record count",
		},
		"partial header shorter than requested": {
			prepare: func(fake *fakeBMC) {
				fake.refuseWholeReads = true
				fake.partialLength = func(offset, requested int) int {
					if offset == 0 {
						return 3
					}
					return requested
				}
			},
			wantErr: "invalid partial SDR",
		},
		"empty partial response": {
			prepare: func(fake *fakeBMC) {
				fake.refuseWholeReads = true
				fake.partialLength = func(offset, requested int) int {
					if offset > 0 {
						return 0
					}
					return requested
				}
			},
			wantErr: "invalid partial SDR",
		},
		"cyclic record chain": {
			prepare: func(b *fakeBMC) {
				b.onExchange = func(req types.Request) error {
					if q, ok := req.(*storage.GetSDRRequest); ok {
						q.RecordID = 0x0000
					}
					return nil
				}
			},
			wantErr: "continues past the repository record count",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fake := newFakeBMC()
			tc.prepare(fake)

			got, err := newTestReader(fake).Collect(t.Context(), false)
			require.ErrorContains(t, err, tc.wantErr)
			assert.Nil(t, got, "no snapshot from an inconsistent inventory")
		})
	}
}

func TestReader_InventoryTolerated(t *testing.T) {
	tests := map[string]struct {
		prepare func(*fakeBMC)
	}{
		"chain ends before the record count": {
			prepare: func(fake *fakeBMC) { fake.recordCount = 5 },
		},
		"whole-record reads refused": {
			prepare: func(fake *fakeBMC) { fake.refuseWholeReads = true },
		},
		"partial reads shorter than requested": {
			prepare: func(fake *fakeBMC) {
				fake.refuseWholeReads = true
				fake.partialLength = func(_, requested int) int { return min(requested, 7) }
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fake := newFakeBMC()
			tc.prepare(fake)

			got, err := newTestReader(fake).Collect(t.Context(), false)
			require.NoError(t, err)
			assert.Equal(t, &Snapshot{
				Sensors:     []Sensor{wantCPUTemp(StateNominal, new(25.0)), wantCPUPresence(StateNominal)},
				CollectedAt: testNow,
			}, got)
		})
	}
}

func TestReader_PartialRecordReadsMaximumRecord(t *testing.T) {
	// An OEM record with the largest body: header and body reach offset 259.
	record := make([]byte, types.SDRRecordHeaderSize+255)
	record[2] = 0x51 // SDR version
	record[3] = 0xc0 // OEM record type
	record[4] = 255  // record body length
	fake := newFakeBMC()
	fake.records = [][]byte{record}
	fake.refuseWholeReads = true

	got, err := newTestReader(fake).Collect(t.Context(), false)
	require.NoError(t, err)
	assert.Empty(t, got.Sensors, "an OEM record is not a sensor")

	requests := fake.sdrRequests()
	require.NotEmpty(t, requests)
	assert.Equal(t, &storage.GetSDRRequest{
		ReservationID: 1,
		RecordID:      0x0000,
		ReadOffset:    0xff,
		ReadBytes:     5,
	}, requests[len(requests)-1], "the last chunk starts at the last addressable offset")
}

func TestReader_PartialRecordReadsPastLastOffset(t *testing.T) {
	// Three-byte responses step past offset FFh before the 260-byte record is complete.
	record := make([]byte, types.SDRRecordHeaderSize+255)
	record[2] = 0x51 // SDR version
	record[3] = 0xc0 // OEM record type
	record[4] = 255  // record body length
	fake := newFakeBMC()
	fake.records = [][]byte{record}
	fake.refuseWholeReads = true
	fake.partialLength = func(offset, requested int) int {
		if offset == 0 {
			return requested
		}
		return min(requested, 3)
	}

	got, err := newTestReader(fake).Collect(t.Context(), false)
	require.ErrorContains(t, err, "past the last addressable offset")
	assert.Nil(t, got)
}
