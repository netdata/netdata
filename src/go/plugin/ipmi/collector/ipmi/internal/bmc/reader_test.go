// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

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
	bmc := newFakeBMC()
	r := newTestReader(bmc)

	require.NoError(t, r.Check(t.Context()))
	assert.Equal(t, []types.Request{&app.GetDeviceIDRequest{}}, bmc.requests, "Check is a single probe")

	got, err := r.Collect(t.Context(), true)
	require.NoError(t, err)
	assert.Equal(t, &Snapshot{
		Sensors:     []Sensor{wantCPUTemp(StateNominal, new(25.0)), wantCPUPresence(StateNominal)},
		SELEntries:  new(fakeSELEntries),
		CollectedAt: testNow,
	}, got)

	require.NoError(t, r.Close(t.Context()))
	require.NoError(t, r.Close(t.Context()))
	assert.Equal(t, 1, bmc.connects)
	assert.Equal(t, 1, bmc.closes)
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
			bmc := newFakeBMC()
			r := newTestReader(bmc)
			before, err := r.Collect(t.Context(), false)
			require.NoError(t, err)

			bmc.readings[10] = tc.reading
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
	bmc := newFakeBMC()
	r := newTestReader(bmc)

	bmc.readingErrs[10] = types.NewResponseError(0xcb, "not present")
	bmc.selErr = errors.New("SEL transport failure")
	got, err := r.Collect(t.Context(), true)
	require.NoError(t, err)
	assert.Equal(t, &Snapshot{
		Sensors:     []Sensor{wantCPUTemp(StateUnknown, nil), wantCPUPresence(StateNominal)},
		CollectedAt: testNow,
		Warnings:    []string{readingWarning, selWarning},
	}, got)

	delete(bmc.readingErrs, 10)
	bmc.selErr = nil
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
			bmc := newFakeBMC()
			r := newTestReader(bmc)
			_, err := r.Collect(t.Context(), false)
			require.NoError(t, err)

			tc.fail(bmc)
			got, err := r.Collect(t.Context(), false)
			require.ErrorContains(t, err, "transport failed")
			assert.Nil(t, got)
			assert.Equal(t, 1, bmc.closes, "a failed collection closes the device")

			// The repository timestamps are sentinels, so only the reconnect makes a rename visible.
			bmc.repoErr = nil
			delete(bmc.readingErrs, 10)
			bmc.records[0] = fullRecord(1, 10, "Renamed")
			got, err = r.Collect(t.Context(), false)
			require.NoError(t, err)
			assert.Equal(t, 2, bmc.connects)
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
			bmc := newFakeBMC()
			r := newTestReader(bmc)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.cancelOn == nil {
				cancel()
			} else {
				bmc.onExchange = func(req types.Request) error {
					if tc.cancelOn(req) {
						cancel()
					}
					return nil
				}
			}

			got, err := r.Collect(ctx, true)
			require.ErrorIs(t, err, context.Canceled)
			assert.Nil(t, got)
			assert.Equal(t, tc.wantCloses, bmc.closes)
		})
	}
}

func TestReader_Collect_CommandDeadlineIdentity(t *testing.T) {
	bmc := newFakeBMC()
	bmc.onExchange = func(req types.Request) error {
		if _, ok := req.(*storage.GetSDRRepoInfoRequest); ok {
			return context.DeadlineExceeded
		}
		return nil
	}

	_, err := newTestReader(bmc).Collect(t.Context(), false)
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

	bmc := newFakeBMC()
	bmc.records = [][]byte{shared, satellite, lun, channel}
	bmc.readings[20] = []byte{0, 0xc0, 0x01, 0} // presence detected
	bmc.readings[21] = []byte{0, 0xc0, 0x02, 0} // failure detected
	bmc.readings[22] = []byte{0, 0xc0, 0x80, 0} // configuration error
	bmc.readings[30] = []byte{30, 0xc0, 0x00}

	got, err := newTestReader(bmc).Collect(t.Context(), false)
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
		Warnings:    []string{"2 sensor records have unsupported ownership, sharing, or ID encoding"},
	}, got)
	assert.Equal(
		t,
		[]uint8{20, 21, 22, 30},
		bmc.sensorReadingRequests(),
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
			bmc := newFakeBMC()
			bmc.records = bmc.records[:1]
			bmc.records[0][fullLinearization] = byte(tc.linearization)
			bmc.records[0][fullExponents] = 0xf0 // R exponent -1
			bmc.readings[10] = []byte{tc.raw, 0xc0, 0x00}
			bmc.onExchange = func(req types.Request) error {
				if _, ok := req.(*sensor.GetSensorReadingFactorsRequest); ok {
					return tc.factorsErr
				}
				return nil
			}

			got, err := newTestReader(bmc).Collect(t.Context(), false)
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

func TestReader_CleanupDeadline(t *testing.T) {
	tests := map[string]struct {
		// cleanup drives one cleanup path; cancelCaller cancels the caller's context when the case requests it.
		cleanup func(t *testing.T, ctx context.Context, r *Reader, conn *closeRecorder, cancelCaller func())
	}{
		"failed connect": {
			cleanup: func(t *testing.T, ctx context.Context, r *Reader, conn *closeRecorder, cancelCaller func()) {
				conn.onConnect = cancelCaller
				conn.connectErr = errors.New("connect failed")
				require.Error(t, r.Check(ctx))
			},
		},
		"failed collection": {
			cleanup: func(t *testing.T, ctx context.Context, r *Reader, conn *closeRecorder, cancelCaller func()) {
				conn.onExchange = func(types.Request) error {
					cancelCaller()
					return errors.New("transport failed")
				}
				_, err := r.Collect(ctx, false)
				require.Error(t, err)
			},
		},
		"explicit close": {
			cleanup: func(t *testing.T, ctx context.Context, r *Reader, _ *closeRecorder, cancelCaller func()) {
				require.NoError(t, r.Check(ctx))
				cancelCaller()
				assert.Equal(t, ctx.Err(), r.Close(ctx), "Close reports the caller's cancellation")
			},
		},
	}

	for name, tc := range tests {
		for _, canceled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/caller canceled=%t", name, canceled), func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				cancelCaller := func() {
					if canceled {
						cancel()
					}
				}
				conn := &closeRecorder{
					fakeBMC: newFakeBMC(),
				}
				r := newTestReader(conn.fakeBMC)
				r.newTransport = func() (transport, error) { return conn, nil }
				// The cleanup budget must not scale with the command timeout.
				r.cfg.Timeout = 24 * time.Hour

				tc.cleanup(t, ctx, r, conn, cancelCaller)

				require.Len(t, conn.closeCalls, 1)
				call := conn.closeCalls[0]
				require.True(t, call.hasDeadline)
				assert.LessOrEqual(t, call.remaining, cleanupTimeout)
				if canceled {
					assert.ErrorIs(t, call.err, context.Canceled, "cleanup keeps the caller's cancellation")
				} else {
					assert.NoError(t, call.err)
				}
			})
		}
	}
}

// closeRecorder records the context of every Close and can fail Connect.
type closeRecorder struct {
	*fakeBMC
	onConnect  func()
	connectErr error
	closeCalls []closeCall
}

type closeCall struct {
	hasDeadline bool
	remaining   time.Duration
	err         error
}

func (c *closeRecorder) Connect(ctx context.Context) error {
	if c.onConnect != nil {
		c.onConnect()
	}
	if c.connectErr != nil {
		return c.connectErr
	}
	return c.fakeBMC.Connect(ctx)
}

func (c *closeRecorder) Close(ctx context.Context) error {
	deadline, ok := ctx.Deadline()
	c.closeCalls = append(c.closeCalls, closeCall{
		hasDeadline: ok,
		remaining:   time.Until(deadline),
		err:         ctx.Err(),
	})
	return ctx.Err()
}
