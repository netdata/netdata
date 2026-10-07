// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"context"
	"errors"
	"testing"

	"github.com/bougou/go-ipmi/pkg/command/app"
	"github.com/bougou/go-ipmi/pkg/command/sensor"
	"github.com/bougou/go-ipmi/pkg/command/storage"
	"github.com/bougou/go-ipmi/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	readingWarning    = "1 sensor readings are unavailable or incomplete"
	conversionWarning = "1 sensor values have unsupported units/conversion or invalid results"
	selWarning        = "SEL entry count unavailable"
)

func TestReader_CheckAndCollect(t *testing.T) {
	fake := newFakeBMC()
	r := newTestReader(fake)

	require.NoError(t, r.Check(t.Context()))
	assert.Equal(t, []types.Request{&app.GetDeviceIDRequest{}}, fake.requests, "Check is a single probe")
	assert.Equal(t, 1, fake.closes, "Check leaves the device closed")

	got, err := r.Collect(t.Context(), true)
	require.NoError(t, err)
	assert.Equal(t, &Snapshot{
		Sensors:     []Sensor{wantCPUTemp(StateNominal, new(25.0)), wantCPUPresence(StateNominal)},
		SELEntries:  new(fakeSELEntries),
		CollectedAt: testNow,
	}, got)
	assert.Equal(t, 2, fake.connects, "Collect reopens the device")

	require.NoError(t, r.Close(t.Context()))
	require.NoError(t, r.Close(t.Context()))
	assert.Equal(t, 2, fake.closes)
}

func TestReader_Collect_ReadingAvailability(t *testing.T) {
	tests := map[string]struct {
		reading  []byte
		want     Sensor
		warnings []string
	}{
		"reading unavailable": {
			reading:  []byte{99, 0xe0, 0x10},
			want:     wantCPUTemp(StateUnknown, nil),
			warnings: []string{readingWarning},
		},
		"scanning disabled": {
			reading:  []byte{99, 0x80, 0x10},
			want:     wantCPUTemp(StateUnknown, nil),
			warnings: []string{readingWarning},
		},
		"event messages disabled, scanning enabled": {
			reading: []byte{30, 0x40, 0x10},
			want:    wantCPUTemp(StateCritical, new(30.0)),
		},
		"state byte missing keeps the numeric reading": {
			reading:  []byte{25, 0xc0},
			want:     wantCPUTemp(StateUnknown, new(25.0)),
			warnings: []string{readingWarning},
		},
		"malformed response": {
			reading:  []byte{25},
			want:     wantCPUTemp(StateUnknown, nil),
			warnings: []string{readingWarning},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fake := newFakeBMC()
			r := newTestReader(fake)
			before, err := r.Collect(t.Context(), false)
			require.NoError(t, err)

			fake.readings[10] = tc.reading
			got, err := r.Collect(t.Context(), false)
			require.NoError(t, err)

			assert.Equal(t, &Snapshot{
				Sensors:     []Sensor{tc.want, wantCPUPresence(StateNominal)},
				CollectedAt: testNow,
				Warnings:    tc.warnings,
			}, got)
			assert.Equal(t, wantCPUTemp(StateNominal, new(25.0)), before.Sensors[0], "earlier snapshot must not change")
		})
	}
}

func TestReader_Collect_PartialFailureAndRecovery(t *testing.T) {
	fake := newFakeBMC()
	r := newTestReader(fake)

	fake.readingErrs[10] = types.NewResponseError(0xcb, "not present")
	fake.selErr = errors.New("SEL transport failure")
	got, err := r.Collect(t.Context(), true)
	require.NoError(t, err)
	assert.Equal(t, &Snapshot{
		Sensors:     []Sensor{wantCPUTemp(StateUnknown, nil), wantCPUPresence(StateNominal)},
		CollectedAt: testNow,
		Warnings:    []string{readingWarning, selWarning},
	}, got)

	delete(fake.readingErrs, 10)
	fake.selErr = nil
	got, err = r.Collect(t.Context(), true)
	require.NoError(t, err)
	assert.Equal(t, &Snapshot{
		Sensors:     []Sensor{wantCPUTemp(StateNominal, new(25.0)), wantCPUPresence(StateNominal)},
		SELEntries:  new(fakeSELEntries),
		CollectedAt: testNow,
	}, got)
}

func TestReader_Collect_FailureReconnects(t *testing.T) {
	tests := map[string]struct {
		fail func(*fakeBMC)
	}{
		"inventory transport failure": {fail: func(b *fakeBMC) { b.repoErr = errors.New("transport failed") }},
		"sensor transport failure":    {fail: func(b *fakeBMC) { b.readingErrs[10] = errors.New("transport failed") }},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fake := newFakeBMC()
			r := newTestReader(fake)
			_, err := r.Collect(t.Context(), false)
			require.NoError(t, err)

			tc.fail(fake)
			got, err := r.Collect(t.Context(), false)
			require.ErrorContains(t, err, "transport failed")
			assert.Nil(t, got)
			assert.Equal(t, 1, fake.closes, "a failed collection closes the device")

			// The repository timestamps are sentinels, so only the reconnect makes a rename visible.
			fake.repoErr = nil
			delete(fake.readingErrs, 10)
			fake.records[0] = fullRecord(1, 10, "Renamed")
			got, err = r.Collect(t.Context(), false)
			require.NoError(t, err)
			assert.Equal(t, 2, fake.connects)
			assert.Equal(t, "Renamed", got.Sensors[0].Name, "a reconnect rediscovers the inventory")
		})
	}
}

func TestReader_Collect_Cancellation(t *testing.T) {
	tests := map[string]struct {
		cancelOn   func(types.Request) bool // nil cancels before Collect
		wantCloses int
	}{
		"before collection": {},
		"during inventory": {
			cancelOn:   func(req types.Request) bool { _, ok := req.(*storage.GetSDRRequest); return ok },
			wantCloses: 1,
		},
		"during sensor reading": {
			cancelOn:   func(req types.Request) bool { _, ok := req.(*sensor.GetSensorReadingRequest); return ok },
			wantCloses: 1,
		},
		"during SEL": {
			cancelOn:   func(req types.Request) bool { _, ok := req.(*storage.GetSELInfoRequest); return ok },
			wantCloses: 1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fake := newFakeBMC()
			r := newTestReader(fake)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.cancelOn == nil {
				cancel()
			} else {
				fake.onExchange = func(req types.Request) error {
					if tc.cancelOn(req) {
						cancel()
					}
					return nil
				}
			}

			got, err := r.Collect(ctx, true)
			require.ErrorIs(t, err, context.Canceled)
			assert.Nil(t, got)
			assert.Equal(t, tc.wantCloses, fake.closes)
			if tc.cancelOn == nil {
				assert.Empty(t, fake.requests, "no command after an earlier cancellation")
			}
		})
	}
}

func TestReader_Collect_CommandDeadlineIdentity(t *testing.T) {
	fake := newFakeBMC()
	fake.onExchange = func(req types.Request) error {
		if _, ok := req.(*storage.GetSDRRepoInfoRequest); ok {
			return context.DeadlineExceeded
		}
		return nil
	}

	_, err := newTestReader(fake).Collect(t.Context(), false)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestReader_Collect_OwnershipAndSharing(t *testing.T) {
	shared := compactRecord(3, 20, 0x08, "PSU")
	shared[compactShareCount] = 3
	shared[compactModifierOffset] = 5
	satellite := fullRecord(4, 50, "Remote")
	satellite[recordOwnerID] = 0x22
	lun := fullRecord(5, 30, "BMC LUN")
	lun[recordOwnerLUN] = 0x01
	channel := fullRecord(6, 40, "Channel")
	channel[recordOwnerLUN] = 0x10

	fake := newFakeBMC()
	fake.records = [][]byte{shared, satellite, lun, channel}
	fake.readings[20] = []byte{0, 0xc0, 0x01, 0} // presence detected
	fake.readings[21] = []byte{0, 0xc0, 0x02, 0} // failure detected
	fake.readings[22] = []byte{0, 0xc0, 0x80, 0} // configuration error
	fake.readings[30] = []byte{30, 0xc0, 0x00}

	got, err := newTestReader(fake).Collect(t.Context(), false)
	require.NoError(t, err)

	powerSupply := func(key, name, state string) Sensor {
		return Sensor{
			Key:       key,
			Name:      name,
			Type:      "Power Supply",
			Component: "Power Supply",
			State:     state,
		}
	}
	temperature := func(key, name, state string, value *float64) Sensor {
		return Sensor{
			Key:       key,
			Name:      name,
			Type:      "Temperature",
			Component: componentOther,
			Unit:      UnitCelsius,
			State:     state,
			Value:     value,
		}
	}
	assert.Equal(t, &Snapshot{
		Sensors: []Sensor{
			powerSupply("i3_n20_t255_u0_PSU 5", "PSU 5", StateNominal),
			powerSupply("i3_n21_t255_u0_PSU 6", "PSU 6", StateCritical),
			powerSupply("i3_n22_t255_u0_PSU 7", "PSU 7", StateWarning),
			temperature("i4_n50_t2_u1_Remote", "Remote", StateUnknown, nil),
			temperature("i5_n30_t2_u1_BMC LUN", "BMC LUN", StateNominal, new(30.0)),
			temperature("i6_n40_t2_u1_Channel", "Channel", StateUnknown, nil),
		},
		CollectedAt: testNow,
		Warnings:    []string{"2 sensors have unsupported ownership, sharing, or ID encoding"},
	}, got)
	assert.Equal(
		t,
		[]uint8{20, 21, 22, 30},
		fake.sensorReadingRequests(),
		"satellite and other-channel sensors are never queried",
	)
}

func TestReader_Collect_Conversion(t *testing.T) {
	tests := map[string]struct {
		linearization types.LinearizationFunc
		raw           byte
		factorsErr    error
		wantValue     float64
		wantNoValue   bool
		wantWarnings  []string
	}{
		"non-linear uses per-reading factors": {
			linearization: types.LinearizationFunc_NonLinear,
			raw:           25,
			wantValue:     5, // 25 * M=2 * 10^-1
		},
		"non-linear factor completion-code error": {
			linearization: types.LinearizationFunc_NonLinear,
			raw:           25,
			factorsErr:    types.NewResponseError(0xc1, "unsupported"),
			wantNoValue:   true,
			wantWarnings:  []string{conversionWarning},
		},
		"EXP10 keeps the fractional exponent": {
			linearization: types.LinearizationFunc_EXP10,
			raw:           15,
			wantValue:     31.622776601683793, // 10^(15 * 10^-1)
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fake := newFakeBMC()
			fake.records = fake.records[:1]
			fake.records[0][fullLinearization] = byte(tc.linearization)
			fake.records[0][fullExponents] = 0xf0 // R exponent -1
			fake.readings[10] = []byte{tc.raw, 0xc0, 0x00}
			fake.onExchange = func(req types.Request) error {
				if _, ok := req.(*sensor.GetSensorReadingFactorsRequest); ok {
					return tc.factorsErr
				}
				return nil
			}

			got, err := newTestReader(fake).Collect(t.Context(), false)
			require.NoError(t, err)
			require.Len(t, got.Sensors, 1)
			assert.Equal(t, StateNominal, got.Sensors[0].State)
			assert.Equal(t, tc.wantWarnings, got.Warnings)
			if tc.wantNoValue {
				assert.Nil(t, got.Sensors[0].Value)
				return
			}
			require.NotNil(t, got.Sensors[0].Value)
			assert.InDelta(t, tc.wantValue, *got.Sensors[0].Value, 1e-12)
		})
	}
}

func TestReader_Check(t *testing.T) {
	tests := map[string]struct {
		// prepare configures the fake; cancel cancels the caller's context.
		prepare      func(fake *fakeBMC, cancel context.CancelFunc)
		wantErr      error
		wantErrMsg   string
		wantRequests int
	}{
		"probe succeeds": {
			wantRequests: 1,
		},
		"probe fails": {
			prepare: func(fake *fakeBMC, _ context.CancelFunc) {
				fake.onExchange = func(types.Request) error { return errors.New("transport failed") }
			},
			wantErrMsg:   "get IPMI device ID: transport failed",
			wantRequests: 1,
		},
		"probe error wins over close error": {
			prepare: func(fake *fakeBMC, _ context.CancelFunc) {
				fake.onExchange = func(types.Request) error { return errors.New("transport failed") }
				fake.closeErr = errors.New("bad descriptor")
			},
			wantErrMsg:   "get IPMI device ID: transport failed",
			wantRequests: 1,
		},
		"close fails": {
			prepare: func(fake *fakeBMC, _ context.CancelFunc) {
				fake.closeErr = errors.New("bad descriptor")
			},
			wantErrMsg:   "close IPMI device: bad descriptor",
			wantRequests: 1,
		},
		"connect fails": {
			prepare: func(fake *fakeBMC, _ context.CancelFunc) {
				fake.onConnect = func() error { return errors.New("permission denied") }
			},
			wantErrMsg: "connect IPMI: permission denied",
		},
		"caller cancellation during connect takes precedence": {
			prepare: func(fake *fakeBMC, cancel context.CancelFunc) {
				fake.onConnect = func() error {
					cancel()
					return errors.New("permission denied")
				}
			},
			wantErr: context.Canceled,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			fake := newFakeBMC()
			if tc.prepare != nil {
				tc.prepare(fake, cancel)
			}

			err := newTestReader(fake).Check(ctx)
			switch {
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
			case tc.wantErrMsg != "":
				require.EqualError(t, err, tc.wantErrMsg)
			default:
				require.NoError(t, err)
			}
			assert.Len(t, fake.requests, tc.wantRequests)
			assert.Equal(t, 1, fake.closes, "Check leaves the device closed")
		})
	}
}
