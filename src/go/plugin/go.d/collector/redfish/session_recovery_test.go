// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/testutil"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectorSessionRecoveryCleanupHonorsCollectionDeadline(t *testing.T) {
	var expire atomic.Bool
	var deletes atomic.Int64
	server := testutil.NewServer(t, testutil.ServerConfig{
		SupportSession: true,
		HandleRequest: func(w http.ResponseWriter, r *http.Request) bool {
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
	assert.Equal(t, int64(1), server.SessionCreates.Load(), "an exhausted cycle must not start a new session")
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
			server := testutil.NewServer(t, testutil.ServerConfig{
				SupportSession: test.auth == "session",
				HandleRequest: func(w http.ResponseWriter, r *http.Request) bool {
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
			client := newTestCollector(t, cfg)
			defer client.Cleanup(t.Context())
			_, err := client.collect(ctx)
			require.NoError(t, err)
			fail.Store(true)
			result, err := client.collect(ctx)
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
	server := testutil.NewServer(t, testutil.ServerConfig{
		SupportSession: true,
		HandleRequest: func(w http.ResponseWriter, r *http.Request) bool {
			switch r.URL.Path {
			case chassis:
				testutil.WriteJSON(w, testutil.Resource(chassis, "Chassis", "Chassis", map[string]any{
					"Sensors": testutil.Link(sensors),
				}))
			case sensors:
				testutil.WriteJSON(w, testutil.Collection(sensors, "Sensor", sensors+"/Energy", sensors+"/Temperature"))
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
				testutil.WriteJSON(w, testutil.Resource(r.URL.Path, "Sensor", "Energy", map[string]any{
					"ReadingType": "EnergyJoules", "ReadingUnits": "J", "Reading": reading,
				}))
			case sensors + "/Temperature":
				if cycle.Load() == 1 && r.Header.Get("X-Auth-Token") == "test-token-1" {
					http.Error(w, "expired", http.StatusUnauthorized)
					return true
				}
				testutil.WriteJSON(w, testutil.Resource(r.URL.Path, "Sensor", "Temperature", map[string]any{
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
	client := newTestCollector(t, cfg)
	defer client.Cleanup(t.Context())
	var previous collectionResult
	for index := range 3 {
		cycle.Store(int64(index))
		result, err := client.collect(t.Context())
		require.NoError(t, err)
		require.True(t, result.Complete)
		assert.Equal(t, "success", result.Metrics.Status)
		var power, temperatures []float64
		for _, reading := range result.Hardware {
			switch reading.Metric {
			case "reading_power_value":
				power = append(power, reading.Value)
			case "reading_temperature_value":
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
	assert.Equal(t, int64(2), server.SessionCreates.Load())
}
