// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/bougou/go-ipmi/pkg/command/sensor"
	"github.com/bougou/go-ipmi/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func thresholdWarning(n int) string {
	return fmt.Sprintf("%d sensor thresholds are unavailable", n)
}

func TestReader_Collect_Thresholds(t *testing.T) {
	// A BMC can keep earlier limits in its SDR after its thresholds change.
	staleSDR := limits{
		lcr: 0,
		lnc: 5,
		unc: 88,
		ucr: 93,
	}
	tests := map[string]struct {
		prepare      func(fake *fakeBMC)
		want         Thresholds
		wantRequests []uint8
		wantWarnings []string
	}{
		"live values replace the SDR values": {
			prepare: func(fake *fakeBMC) {
				mask := uint8(maskLCR | maskLNC | maskUNC | maskUCR)
				setThresholds(fake.records[0], types.SensorThresholdAccess_ReadableSettable, mask, mask, staleSDR)
				fake.thresholds[10] = thresholdsResponse(
					mask,
					limits{
						lnr: 7,
						lcr: 0,
						lnc: 5,
						unc: 97,
						ucr: 100,
						unr: 120,
					},
				)
			},
			want: Thresholds{
				LowerCritical:    new(0.0),
				LowerNonCritical: new(5.0),
				UpperNonCritical: new(97.0),
				UpperCritical:    new(100.0),
			},
			wantRequests: []uint8{10},
		},
		"the response mask decides which thresholds exist": {
			prepare: func(fake *fakeBMC) {
				setThresholds(fake.records[0], types.SensorThresholdAccess_Readable, maskUNC, 0, staleSDR)
				fake.thresholds[10] = thresholdsResponse(maskLNR|maskUNR, limits{
					lnr: 1,
					lcr: 2,
					unc: 3,
					unr: 110,
				})
			},
			want: Thresholds{
				LowerNonRecoverable: new(1.0),
				UpperNonRecoverable: new(110.0),
			},
			wantRequests: []uint8{10},
		},
		"fixed thresholds come from the SDR readable mask": {
			prepare: func(fake *fakeBMC) {
				// Fixed thresholds are not settable; a settable mask read as readable shows none.
				setThresholds(fake.records[0], types.SensorThresholdAccess_Fixed, maskUNC|maskUCR, 0, staleSDR)
			},
			want: Thresholds{
				UpperNonCritical: new(88.0),
				UpperCritical:    new(93.0),
			},
		},
		"no threshold access sends nothing": {
			prepare: func(fake *fakeBMC) {
				mask := uint8(maskUNC | maskUCR)
				setThresholds(fake.records[0], types.SensorThresholdAccess_No, mask, mask, staleSDR)
				fake.thresholds[10] = thresholdsResponse(mask, staleSDR)
			},
		},
		"completion-code error leaves the sensor without thresholds": {
			prepare: func(fake *fakeBMC) {
				setThresholds(fake.records[0], types.SensorThresholdAccess_Readable, maskUNC, 0, staleSDR)
			},
			wantRequests: []uint8{10},
			wantWarnings: []string{thresholdWarning(1)},
		},
		"non-linear sensor has none": {
			prepare: func(fake *fakeBMC) {
				setThresholds(fake.records[0], types.SensorThresholdAccess_Readable, maskUNC, 0, staleSDR)
				fake.records[0][fullLinearization] = byte(types.LinearizationFunc_NonLinear)
				fake.thresholds[10] = thresholdsResponse(maskUNC, staleSDR)
			},
		},
		"sensor without a supported unit has none": {
			prepare: func(fake *fakeBMC) {
				setThresholds(fake.records[0], types.SensorThresholdAccess_Readable, maskUNC, 0, staleSDR)
				fake.records[0][fullBaseUnit] = byte(types.SensorUnitType_Unspecified)
				fake.thresholds[10] = thresholdsResponse(maskUNC, staleSDR)
			},
			wantWarnings: []string{conversionWarning},
		},
		"discrete full record has none": {
			prepare: func(fake *fakeBMC) {
				setThresholds(fake.records[0], types.SensorThresholdAccess_Readable, maskUNC, 0, staleSDR)
				fake.records[0][fullEventReadingType] = 0x6f // sensor-specific, with a numeric reading
				fake.thresholds[10] = thresholdsResponse(maskUNC, staleSDR)
			},
		},
		"unsupported record is never queried": {
			prepare: func(fake *fakeBMC) {
				setThresholds(fake.records[0], types.SensorThresholdAccess_Readable, maskUNC, 0, staleSDR)
				fake.records[0][recordOwnerID] = 0x30 // satellite controller
				fake.thresholds[10] = thresholdsResponse(maskUNC, staleSDR)
			},
			wantWarnings: []string{"1 sensors have unsupported ownership, sharing, or ID encoding"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fake := newFakeBMC()
			fake.thresholds = map[uint8][]byte{}
			tc.prepare(fake)

			got, err := newTestReader(fake).Collect(t.Context(), false)
			require.NoError(t, err)

			require.Len(t, got.Sensors, 2)
			assert.Equal(t, tc.want, got.Sensors[0].Thresholds)
			assert.Equal(t, Thresholds{}, got.Sensors[1].Thresholds, "discrete sensors have no thresholds")
			assert.Equal(t, tc.wantRequests, fake.thresholdRequests())
			assert.Equal(t, tc.wantWarnings, got.Warnings)
		})
	}
}

func TestReader_Collect_ThresholdConversion(t *testing.T) {
	tests := map[string]struct {
		format        byte // analog data format in bits 7:6
		m, b          byte
		linearization types.LinearizationFunc
		reading       byte
		raw           limits
		mask          uint8
		wantValue     *float64
		want          Thresholds
	}{
		"signed values with offset": {
			format:  0x80, // two's complement
			m:       2,
			b:       3,
			reading: 0xf6, // -10
			raw: limits{
				lcr: 0xec,
				ucr: 40,
			},
			mask:      maskLCR | maskUCR,
			wantValue: new(-17.0), // 2 * -10 + 3
			want: Thresholds{
				LowerCritical: new(-37.0), // 2 * -20 + 3
				UpperCritical: new(83.0),  // 2 * 40 + 3
			},
		},
		"linearization applies": {
			m:             1,
			linearization: types.LinearizationFunc_SQR,
			reading:       5,
			raw: limits{
				unc: 9,
			},
			mask:      maskUNC,
			wantValue: new(25.0),
			want: Thresholds{
				UpperNonCritical: new(81.0),
			},
		},
		"non-finite values are left out": {
			m:             1,
			linearization: types.LinearizationFunc_1X,
			reading:       2,
			raw: limits{
				lnc: 0,
				unc: 4,
			},
			mask:      maskLNC | maskUNC,
			wantValue: new(0.5),
			want: Thresholds{
				UpperNonCritical: new(0.25),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fake := newFakeBMC()
			record := fake.records[0]
			record[fullUnitsFormat] = tc.format
			record[fullM] = tc.m
			record[fullB] = tc.b
			record[fullLinearization] = byte(tc.linearization)
			setThresholds(record, types.SensorThresholdAccess_Readable, tc.mask, 0, limits{})
			fake.readings[10] = []byte{tc.reading, 0xc0, 0x00}
			fake.thresholds = map[uint8][]byte{10: thresholdsResponse(tc.mask, tc.raw)}

			got, err := newTestReader(fake).Collect(t.Context(), false)
			require.NoError(t, err)

			want := wantCPUTemp(StateNominal, tc.wantValue)
			want.Thresholds = tc.want
			assert.Equal(t, want, got.Sensors[0], "thresholds use the reading's conversion")
		})
	}
}

func TestReader_Collect_ThresholdFailures(t *testing.T) {
	inletThresholds := limits{
		unc: 40,
		ucr: 45,
	}
	temperature := func(key, name string, thresholds Thresholds) Sensor {
		s := wantCPUTemp(StateNominal, new(25.0))
		s.Key, s.Name, s.Component = key, name, "Other"
		s.Thresholds = thresholds
		return s
	}
	inlet := func(thresholds Thresholds) Sensor {
		return temperature("i3_n12_t2_u1_Inlet Temp", "Inlet Temp", thresholds)
	}
	// Fixed thresholds come from the SDR, so a stop does not affect them.
	fixed := temperature("i4_n13_t2_u1_Fixed Temp", "Fixed Temp", Thresholds{
		UpperCritical: new(70.0),
	})
	tests := map[string]struct {
		thresholdsErr error // returned for sensor 10
		wantRequests  []uint8
		wantInlet     Sensor
		wantWarnings  []string
	}{
		"completion-code error continues with the next sensor": {
			thresholdsErr: types.NewResponseError(types.CodeUnspecifiedError, "unspecified"),
			wantRequests:  []uint8{10, 12},
			wantInlet: inlet(Thresholds{
				UpperNonCritical: new(40.0),
				UpperCritical:    new(45.0),
			}),
			wantWarnings: []string{thresholdWarning(1)},
		},
		"transport failure stops the threshold reads": {
			thresholdsErr: errors.New("timeout"),
			wantRequests:  []uint8{10},
			wantInlet:     inlet(Thresholds{}),
			wantWarnings:  []string{thresholdWarning(2)},
		},
		"malformed response stops the threshold reads": {
			thresholdsErr: types.NewResponseError(0, "short response"),
			wantRequests:  []uint8{10},
			wantInlet:     inlet(Thresholds{}),
			wantWarnings:  []string{thresholdWarning(2)},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fake := newFakeBMC()
			fake.records = append(fake.records, fullRecord(3, 12, "Inlet Temp"), fullRecord(4, 13, "Fixed Temp"))
			fake.readings[12] = []byte{25, 0xc0, 0x00}
			fake.readings[13] = []byte{25, 0xc0, 0x00}
			setThresholds(fake.records[0], types.SensorThresholdAccess_Readable, maskUNC, 0, limits{})
			setThresholds(fake.records[2], types.SensorThresholdAccess_Readable, maskUNC|maskUCR, 0, limits{})
			setThresholds(fake.records[3], types.SensorThresholdAccess_Fixed, maskUCR, 0, limits{
				ucr: 70,
			})
			fake.thresholds = map[uint8][]byte{
				10: thresholdsResponse(maskUNC, limits{
					unc: 80,
				}),
				12: thresholdsResponse(maskUNC|maskUCR, inletThresholds),
			}
			fake.onExchange = func(req types.Request) error {
				if q, ok := req.(*sensor.GetSensorThresholdsRequest); ok && q.SensorNumber == 10 {
					return tc.thresholdsErr
				}
				return nil
			}

			r := newTestReader(fake)
			want := &Snapshot{
				Sensors: []Sensor{
					wantCPUTemp(StateNominal, new(25.0)),
					wantCPUPresence(StateNominal),
					tc.wantInlet,
					fixed,
				},
				CollectedAt: testNow,
				Warnings:    tc.wantWarnings,
			}
			got, err := r.Collect(t.Context(), false)
			require.NoError(t, err, "threshold failures never fail the collection")
			assert.Equal(t, want, got, "states and readings are unaffected")

			got, err = r.Collect(t.Context(), false)
			require.NoError(t, err)
			assert.Equal(t, want, got, "collections between refreshes report the same thresholds and warning")
			assert.Equal(t, tc.wantRequests, fake.thresholdRequests())
			assert.Zero(t, fake.closes, "the connection stays open")
		})
	}
}

func TestReader_Collect_ThresholdCancellation(t *testing.T) {
	fake := newFakeBMC()
	setThresholds(fake.records[0], types.SensorThresholdAccess_Readable, maskUNC, 0, limits{})
	fake.thresholds = map[uint8][]byte{10: thresholdsResponse(maskUNC, limits{
		unc: 80,
	})}
	r := newTestReader(fake)

	ctx, cancel := context.WithCancel(t.Context())
	fake.onExchange = func(req types.Request) error {
		if _, ok := req.(*sensor.GetSensorThresholdsRequest); ok {
			cancel()
		}
		return nil
	}
	got, err := r.Collect(ctx, false)
	require.ErrorIs(t, err, context.Canceled, "a cancellation during threshold reads fails the collection")
	assert.Nil(t, got)

	fake.onExchange = nil
	got, err = r.Collect(t.Context(), false)
	require.NoError(t, err)
	assert.Equal(t, Thresholds{
		UpperNonCritical: new(80.0),
	}, got.Sensors[0].Thresholds,
		"the next collection rediscovers the inventory")
	assert.Equal(t, []uint8{10, 10}, fake.thresholdRequests())
}

func TestReader_Collect_ThresholdsRefreshWithInventory(t *testing.T) {
	fake := newFakeBMC()
	setThresholds(fake.records[0], types.SensorThresholdAccess_Readable, maskUNC, 0, limits{})
	fake.thresholds = map[uint8][]byte{10: thresholdsResponse(maskUNC, limits{
		unc: 80,
	})}
	r := newTestReader(fake)
	now := testNow
	r.now = func() time.Time { return now }

	collect := func() Thresholds {
		t.Helper()
		got, err := r.Collect(t.Context(), false)
		require.NoError(t, err)
		return got.Sensors[0].Thresholds
	}

	assert.Equal(t, Thresholds{
		UpperNonCritical: new(80.0),
	}, collect())
	fake.thresholds[10] = thresholdsResponse(maskUNC, limits{
		unc: 90,
	})
	assert.Equal(t, Thresholds{
		UpperNonCritical: new(80.0),
	}, collect(), "cached with the inventory")
	assert.Equal(t, []uint8{10}, fake.thresholdRequests())

	now = now.Add(inventoryMaxAge)
	assert.Equal(t, Thresholds{
		UpperNonCritical: new(90.0),
	}, collect(), "reread with the inventory")
	assert.Equal(t, []uint8{10, 10}, fake.thresholdRequests())

	// A failed collection reconnects, and the next one rediscovers the inventory.
	fake.thresholds[10] = thresholdsResponse(maskUNC, limits{
		unc: 95,
	})
	fake.readingErrs[11] = errors.New("transport failed")
	_, err := r.Collect(t.Context(), false)
	require.Error(t, err)
	delete(fake.readingErrs, 11)
	assert.Equal(t, Thresholds{
		UpperNonCritical: new(95.0),
	}, collect(), "reread after a reconnect")
	assert.Equal(t, []uint8{10, 10, 10}, fake.thresholdRequests())
}
