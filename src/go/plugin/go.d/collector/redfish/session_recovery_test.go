// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectorSessionRecoveryCleanupHonorsCollectionDeadline(t *testing.T) {
	var expire atomic.Bool
	var deletes atomic.Int64
	server := newRedfishTestServer(t, redfishTestServerConfig{
		supportSession: true,
		handleRequest: func(w http.ResponseWriter, r *http.Request) bool {
			if r.Method == http.MethodDelete {
				deletes.Add(1)
				<-r.Context().Done()
				return true
			}
			if expire.Load() && r.URL.Path == "/redfish/v1/" && r.Header.Get("X-Auth-Token") != "" {
				http.Error(w, "expired", http.StatusUnauthorized)
				return true
			}
			return false
		},
	})
	defer server.Close()
	collector := New()
	collector.Config = testConfig(server.URL, "session")
	collector.Name = "recovery-deadline"
	collector.UpdateEvery = 1
	collector.Timeout = confopt.Duration(3 * time.Second)
	require.NoError(t, collector.Init(t.Context()))
	defer collector.Cleanup(t.Context())
	sourceTestCollectCycle(t, collector)
	expire.Store(true)

	started := time.Now()
	sourceTestCollectCycle(t, collector)
	assert.Less(t, time.Since(started), 2*time.Second, "cleanup must stop at the one-second collection deadline")
	assert.Equal(t, int64(1), deletes.Load(), "the recovery logout reached the BMC")
	assert.Equal(t, int64(1), server.sessionCreates.Load(), "an exhausted cycle must not start a new session")
}

func TestSessionRecoveryCleanupHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var expire atomic.Bool
	var deletes atomic.Int64
	server := newRedfishTestServer(t, redfishTestServerConfig{
		supportSession: true,
		handleRequest: func(w http.ResponseWriter, r *http.Request) bool {
			if r.Method == http.MethodDelete {
				deletes.Add(1)
				cancel()
				<-r.Context().Done()
				return true
			}
			if expire.Load() && r.URL.Path == "/redfish/v1/" && r.Header.Get("X-Auth-Token") != "" {
				http.Error(w, "expired", http.StatusUnauthorized)
				return true
			}
			return false
		},
	})
	defer server.Close()
	cfg := testConfig(server.URL, "session")
	cfg.Timeout = confopt.Duration(2 * time.Second)
	client := newTestProtocolClient(t, cfg)
	defer client.Close()
	_, err := client.Collect(ctx)
	require.NoError(t, err)
	expire.Store(true)

	started := time.Now()
	result, err := client.Collect(ctx)
	require.ErrorIs(t, err, context.Canceled)
	assert.Less(t, time.Since(started), time.Second, "canceling collection must interrupt recovery logout")
	assert.Equal(t, "unavailable", result.Metrics.Status)
	assert.Nil(t, client.sdk)
	assert.Equal(t, int64(1), deletes.Load())
	assert.Equal(t, int64(1), server.sessionCreates.Load())
}

func TestSessionRecoveryStopsAfterOneAttempt(t *testing.T) {
	for name, test := range map[string]struct {
		auth        string
		readStatus  int
		loginStatus int
		cancel      bool
		wantLogins  int64
	}{
		"replacement session rejected":     {auth: "session", readStatus: 401, wantLogins: 2},
		"replacement credentials rejected": {auth: "session", readStatus: 401, loginStatus: 401, wantLogins: 2},
		"replacement login unavailable":    {auth: "session", readStatus: 401, loginStatus: 503, wantLogins: 2},
		"canceled collection":              {auth: "session", readStatus: 401, cancel: true, wantLogins: 1},
		"forbidden":                        {auth: "session", readStatus: 403, wantLogins: 1},
		"server error":                     {auth: "session", readStatus: 500, wantLogins: 1},
		"basic unauthorized":               {auth: "basic", readStatus: 401},
		"unauthenticated unauthorized":     {auth: "none", readStatus: 401},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var fail atomic.Bool
			var logins atomic.Int64
			server := newRedfishTestServer(t, redfishTestServerConfig{
				supportSession: test.auth == "session",
				handleRequest: func(w http.ResponseWriter, r *http.Request) bool {
					if r.Method == http.MethodPost {
						logins.Add(1)
						if fail.Load() && test.loginStatus != 0 {
							http.Error(w, "login failed", test.loginStatus)
							return true
						}
					}
					if fail.Load() && r.Method == http.MethodGet && r.URL.Path == "/redfish/v1/" &&
						(test.auth != "session" || r.Header.Get("X-Auth-Token") != "") {
						http.Error(w, "read failed", test.readStatus)
						if test.cancel {
							cancel()
						}
						return true
					}
					return false
				},
			})
			defer server.Close()
			cfg := testConfig(server.URL, test.auth)
			client := newTestProtocolClient(t, cfg)
			defer client.Close()
			_, err := client.Collect(ctx)
			require.NoError(t, err)
			fail.Store(true)
			result, err := client.Collect(ctx)
			require.Error(t, err)
			assert.Equal(t, "unavailable", result.Metrics.Status)
			assert.Empty(t, result.Hardware)
			assert.Equal(t, test.wantLogins, logins.Load())
			if test.cancel {
				require.ErrorIs(t, err, context.Canceled)
			}
		})
	}
}

func TestSessionRecoveryProjectsOnlyFinalAcquisition(t *testing.T) {
	const chassis = "/redfish/v1/Chassis/1"
	const sensors = chassis + "/Sensors"
	var cycle atomic.Int64
	var energyReads atomic.Int64
	server := newRedfishTestServer(t, redfishTestServerConfig{
		supportSession: true,
		handleRequest: func(w http.ResponseWriter, r *http.Request) bool {
			switch r.URL.Path {
			case chassis:
				writeJSON(w, sourceTestResource(chassis, "Chassis", "Chassis", map[string]any{
					"Sensors": sourceTestLink(sensors),
				}))
			case sensors:
				writeJSON(w, sourceTestCollection(sensors, "Sensor", sensors+"/Energy", sensors+"/Temperature"))
			case sensors + "/Energy":
				energyReads.Add(1)
				reading := int64(100)
				if cycle.Load() == 1 {
					reading = 200
					if r.Header.Get("X-Auth-Token") == "test-token-2" {
						reading = 400
					}
				} else if cycle.Load() == 2 {
					reading = 700
				}
				writeJSON(w, sourceTestResource(r.URL.Path, "Sensor", "Energy", map[string]any{
					"ReadingType": "EnergyJoules", "ReadingUnits": "J", "Reading": reading,
				}))
			case sensors + "/Temperature":
				if cycle.Load() == 1 && r.Header.Get("X-Auth-Token") == "test-token-1" {
					http.Error(w, "expired", http.StatusUnauthorized)
					return true
				}
				writeJSON(w, sourceTestResource(r.URL.Path, "Sensor", "Temperature", map[string]any{
					"ReadingType": "Temperature", "ReadingUnits": "Cel", "Reading": 30,
				}))
			default:
				return false
			}
			return true
		},
	})
	defer server.Close()
	cfg := testConfig(server.URL, "session")
	cfg.MaxConcurrentRequests = 1 // Read energy before the later session failure.
	client := newTestProtocolClient(t, cfg)
	defer client.Close()
	var previous collectionResult
	for index := range 3 {
		cycle.Store(int64(index))
		result, err := client.Collect(t.Context())
		require.NoError(t, err)
		require.True(t, result.Complete)
		assert.Equal(t, "success", result.Metrics.Status)
		var power, temperatures []float64
		for _, reading := range result.Hardware {
			switch reading.Metric {
			case "reading_power_value":
				power = append(power, reading.Value)
			case "system_hw_sensor_temperature_input":
				temperatures = append(temperatures, reading.Value)
			}
		}
		assert.Equal(t, []float64{30}, temperatures)
		if index == 0 {
			assert.Empty(t, power, "the first sample establishes a rate baseline")
		} else {
			require.Len(t, power, 1, "only the final acquisition is projected")
			want := 300 / result.ObservedAt.Sub(previous.ObservedAt).Seconds()
			assert.InDelta(t, want, power[0], want*0.000001)
		}
		if index == 1 {
			assert.Equal(t, int64(3), energyReads.Load(), "the expired attempt read energy before reacquisition")
			assert.Equal(t, map[string]int{"auth": 1}, result.Metrics.Failures)
		}
		previous = result
	}
	assert.Equal(t, int64(2), server.sessionCreates.Load())
}
