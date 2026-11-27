// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	azcloud "github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryExecutorRetainsThrottleWhenSDKReturnsCancellation(t *testing.T) {
	t.Run("Metrics", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		cred := new(fake.TokenCredential)
		transport := cancelThrottleTransport(cancel, 529)
		executor := newQueryExecutor(1, time.Second, cred, azcloud.AzurePublic, func(endpoint string, cred azcore.TokenCredential, _ azcloud.Configuration) (metricsQueryClient, error) {
			opts := withAzureRequestTransport(azcore.ClientOptions{Transport: transport})
			return azmetrics.NewClient(endpoint, cred, &azmetrics.ClientOptions{ClientOptions: opts})
		})
		ctx = policy.WithRetryOptions(ctx, policy.RetryOptions{ShouldRetry: retryPrimaryMetrics})
		batch := testWireEvidenceBatch("A")
		result := executor.executeQueryBatch(ctx, batch, time.Now(), 0, false)

		require.ErrorIs(t, result.Err, context.Canceled)
		var response *azcore.ResponseError
		require.ErrorAs(t, result.Err, &response)
		assert.Equal(t, 529, response.StatusCode)
		assert.Equal(t, "180", response.RawResponse.Header.Get("Retry-After"))
	})
	t.Run("ARM", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		transport := cancelThrottleTransport(cancel, http.StatusTooManyRequests)
		armClient, err := arm.NewClient("test", "v1.0.0", new(fake.TokenCredential), &arm.ClientOptions{
			ClientOptions: withAzureRequestTransport(azcore.ClientOptions{Transport: transport}),
		})
		require.NoError(t, err)
		batch := testWireEvidenceBatch("A")
		key := definitionKey{batch.Resources[0].ID, batch.Profile.MetricNamespace}
		_, err = (&metricDefinitionsClient{armClient}).list(ctx, key)

		require.ErrorIs(t, err, context.Canceled)
		var response *azcore.ResponseError
		require.ErrorAs(t, err, &response)
		assert.Equal(t, http.StatusTooManyRequests, response.StatusCode)
		assert.Equal(t, "180", response.RawResponse.Header.Get("Retry-After"))
	})
}

func TestRetryAfterUsesRawHeaderReceiptOnBothAzureAPIs(t *testing.T) {
	for _, plane := range []string{"Metrics", "ARM"} {
		t.Run(plane, func(t *testing.T) {
			for _, tc := range []struct {
				name, header string
				wantUntil    time.Duration
			}{
				{name: "relative deadline", header: "50", wantUntil: 55 * time.Second},
				{name: "elapsed during body read", header: "15"},
				{name: "immediate retry", header: "0"},
				{name: "missing header", wantUntil: 65 * time.Second},
			} {
				t.Run(tc.name, func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						// Keep the job's time distinct from the transport's wall clock.
						now := func() time.Time { return time.Now().Add(4 * time.Hour) }
						start := now()
						ctx := withCollectorClock(t.Context(), now)
						batch := testWireEvidenceBatch("A")
						cred := new(fake.TokenCredential)
						var holds serviceHolds
						wrapped := withAzureRequestTransport(azcore.ClientOptions{Transport: azureTestTransport(func(req *http.Request) (*http.Response, error) {
							time.Sleep(5 * time.Second)
							header := make(http.Header)
							if tc.header != "" {
								header.Set("Retry-After", tc.header)
							}
							return &http.Response{
								StatusCode: http.StatusTooManyRequests,
								Header:     header,
								Body:       io.NopCloser(stalledThrottleBody{req.Context()}),
								Request:    req,
							}, nil
						})})
						var isHeld func(time.Time) bool
						switch plane {
						case "Metrics":
							ctx = policy.WithRetryOptions(ctx, policy.RetryOptions{ShouldRetry: retryPrimaryMetrics})
							executor := newQueryExecutor(1, 30*time.Second, cred, azcloud.AzurePublic, func(endpoint string, cred azcore.TokenCredential, _ azcloud.Configuration) (metricsQueryClient, error) {
								return azmetrics.NewClient(endpoint, cred, &azmetrics.ClientOptions{ClientOptions: wrapped})
							})
							result := executor.executeQueryBatch(ctx, batch, start, 0, false)
							require.ErrorIs(t, result.Err, context.DeadlineExceeded)
							holds.recordMetrics(batch, throttleFromError(result.Err, now()))
							isHeld = func(at time.Time) bool { return holds.metricMask(batch, at) != 0 }
						case "ARM":
							ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
							defer cancel()
							client, err := arm.NewClient("test", "v1.0.0", cred, &arm.ClientOptions{ClientOptions: wrapped})
							require.NoError(t, err)
							key := definitionKey{batch.Resources[0].ID, batch.Profile.MetricNamespace}
							_, err = (&metricDefinitionsClient{client}).list(ctx, key)
							require.ErrorIs(t, err, context.DeadlineExceeded)
							holds.recordARM(key, batch.SubscriptionID, throttleFromError(err, now()))
							isHeld = func(at time.Time) bool { return holds.armHeld(key, batch.SubscriptionID, at) }
						}
						require.Equal(t, 30*time.Second, now().Sub(start))
						if tc.wantUntil == 0 {
							assert.False(t, isHeld(now()), "explicit deadline expired while the body was in flight")
						} else {
							deadline := start.Add(tc.wantUntil)
							assert.True(t, isHeld(deadline.Add(-time.Second)))
							assert.False(t, isHeld(deadline))
						}
					})
				})
			}
		})
	}
}

type stalledThrottleBody struct{ ctx context.Context }

func (b stalledThrottleBody) Read([]byte) (int, error) {
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}

func TestQueryExecutorChargesSupplementalPostReplays(t *testing.T) {
	for _, replay := range []string{"claims challenge", "POST redirect", "transport rewind"} {
		t.Run(replay, func(t *testing.T) {
			for _, wantCalls := range []int{1, 2} {
				name := "one slot"
				if wantCalls == 2 {
					name = "two slots"
				}
				t.Run(name, func(t *testing.T) {
					p := queryPoll{
						c:                &Collector{now: time.Now},
						state:            &recoveryState{},
						events:           make(chan queryEvent, 1),
						supplementalUsed: maxRecoveryMetricsCalls - wantCalls + 1, // Includes the first POST's dispatch reservation.
					}
					batch := testWireEvidenceBatch("A")
					op := p.takeOperation()
					op.request = plannedRequest{batch: batch, questions: []*metricQuestion{{batch: batch}}}
					ctx := context.WithValue(t.Context(), queryOperationKey{}, op)
					ctx = policy.WithRetryOptions(ctx, policy.RetryOptions{MaxRetries: -1})

					calls := 0
					var transport policy.Transporter = azureTestTransport(func(req *http.Request) (*http.Response, error) {
						calls++
						assert.Equal(t, http.MethodPost, req.Method)
						var body azmetrics.ResourceIDList
						if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
							return nil, err
						}
						assert.Equal(t, []string{batch.Resources[0].ID}, body.ResourceIDs)
						if calls != 1 {
							return azureTestResponse(req, http.StatusOK, azmetrics.MetricResults{})
						}
						switch replay {
						case "claims challenge":
							resp, err := azureTestResponse(req, http.StatusUnauthorized, nil)
							resp.Header.Set("WWW-Authenticate", `Bearer error="insufficient_claims", claims="e30="`)
							return resp, err
						case "transport rewind":
							retry, err := req.GetBody()
							if err != nil {
								return nil, err
							}
							defer retry.Close()
							calls++
							var replayed azmetrics.ResourceIDList
							if err := json.NewDecoder(retry).Decode(&replayed); err != nil {
								return nil, err
							}
							assert.Equal(t, body, replayed)
							return azureTestResponse(req, http.StatusOK, azmetrics.MetricResults{})
						default:
							resp, err := azureTestResponse(req, http.StatusTemporaryRedirect, nil)
							resp.Header.Set("Location", "/redirected-getBatch")
							return resp, err
						}
					})
					if replay != "claims challenge" {
						transport = &http.Client{Transport: wireEvidenceRoundTripper{transport}}
					}
					cred := new(fake.TokenCredential)
					executor := newQueryExecutor(
						1,
						time.Second,
						cred,
						azcloud.AzurePublic,
						func(endpoint string, cred azcore.TokenCredential, _ azcloud.Configuration) (metricsQueryClient, error) {
							opts := withAzureRequestTransport(azcore.ClientOptions{Transport: transport})
							return azmetrics.NewClient(endpoint, cred, &azmetrics.ClientOptions{ClientOptions: opts})
						},
					)
					completed := make(chan queryBatchResult, 1)
					go func() { completed <- executor.executeQueryBatch(ctx, batch, time.Now(), 0, false) }()
					var result queryBatchResult
				awaitResult:
					for {
						select {
						case e := <-p.events:
							p.onEvent(e)
						case result = <-completed:
							break awaitResult
						}
					}

					if wantCalls == 1 {
						assert.ErrorIs(t, result.Err, errSupplementalBudgetExhausted)
					} else {
						assert.NoError(t, result.Err)
					}
					assert.Equal(t, wantCalls, calls)
					assert.Equal(t, maxRecoveryMetricsCalls, p.supplementalUsed)
				})
			}
		})
	}
}

type wireEvidenceRoundTripper struct{ policy.Transporter }

func (rt wireEvidenceRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt.Do(req)
}

func cancelThrottleTransport(cancel context.CancelFunc, status int) azureTestTransport {
	return func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Header:     http.Header{"Retry-After": {"180"}},
			Body:       io.NopCloser(cancelThrottleBody{req.Context(), cancel}),
			Request:    req,
		}, nil
	}
}

type cancelThrottleBody struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func (b cancelThrottleBody) Read([]byte) (int, error) {
	b.cancel()
	return 0, b.ctx.Err()
}

func TestRejectedMetricsDistinguishesRequestValidationFromMetricIncompatibility(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		code       string
		target     string
		message    string
		canIsolate bool
	}{
		{name: "unavailable metric", status: 400, code: "BadRequest", message: "Failed to find metric configuration for B", canIsolate: true},
		{name: "ambiguous", status: 400, code: "BadRequest", canIsolate: true},
		{name: "invalid time parameters", status: 400, code: "BadRequest", message: "Detected invalid time range parameters for API version 2023-10-01."},
		{name: "window outside retention", status: 400, code: "BadRequest", message: "Query endTime: 06/06/2021 00:00:00 is out of the Max metrics retention period: 93.00:00:00"},
		{name: "invalid subscription code", status: 400, code: "InvalidSubscriptionId"},
		{name: "request-field target", status: 400, code: "BadRequest", target: "starttime"},
		{name: "authorization", status: 403, code: "AuthorizationFailed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := azureTestResponse(nil, tc.status, map[string]any{"error": map[string]string{
				"code": tc.code, "target": tc.target, "message": tc.message,
			}})
			require.NoError(t, err)
			assert.Equal(t, tc.canIsolate, rejectedMetrics(runtime.NewResponseError(resp)))
		})
	}
}

func TestReduceQueryResponseSeparatesMetricErrorsFromEmptyAndMeasured(t *testing.T) {
	batch := testWireEvidenceBatch("A", "B", "C", "D", "E")
	res := batch.Resources[0]
	metricWithError := func(name, code, message string) azmetrics.Metric {
		metric := metricWithTotal(name, time.Now(), 100)
		metric.ErrorCode = new(code)
		metric.ErrorMessage = new(message)
		return metric
	}
	result := reduceQueryResponse([]azmetrics.MetricData{
		{ResourceID: &res.ID, Values: []azmetrics.Metric{
			metricWithTotal("A", time.Now(), 1),
			metricWithError("B", "InvalidSamplingType", "grain unsupported"),
			{Name: &azmetrics.LocalizableString{Value: new("C")}, ErrorCode: new("Success")},
		}},
		{ResourceID: &res.ID, Values: []azmetrics.Metric{
			metricWithError("A", "InvalidSamplingType", "duplicate conflicting row"),
			metricWithError("D", "AuthorizationFailed", "access denied"),
			metricWithError("E", "", "explicit failure without a code"),
		}},
	}, batch.Profile, queryBatchMetricIndex(batch), map[string]resourceInfo{res.idKey: res})
	require.Len(t, result.Samples, 1)
	assert.Equal(t, "A", result.Samples[0].Instrument)
	assert.EqualValues(t, 1, result.Samples[0].Value)
	assert.Equal(t, map[string]uint32{res.idKey: 1 << 1}, result.incompatible)
	assert.Equal(t, map[string]uint32{res.idKey: 1<<1 | 1<<3 | 1<<4}, result.metricErrors)
}

func TestServiceHoldsBackOffHeaderlessThrottlesAndMatchWireIdentity(t *testing.T) {
	batch := testWireEvidenceBatch("A", "B")
	batch.mask = 1 << 1
	var holds serviceHolds
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	at := start
	for _, delay := range []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 5 * time.Minute, 5 * time.Minute} {
		holds.expire(at)
		holds.recordMetrics(batch, throttleReceipt{status: 529, at: at})
		// Rediscovery and another compiled ordering leave the wire name unchanged.
		rediscovered := testWireEvidenceBatch("b", "a")
		rediscovered.Resources[0].ID = strings.ToUpper(rediscovered.Resources[0].ID)
		assert.EqualValues(t, 1, holds.metricMask(rediscovered, at.Add(delay-time.Second)))
		assert.Zero(t, holds.metricMask(rediscovered, at.Add(delay)))
		assert.False(t, holds.armHeld(batch.Resources[0].definitionKey(batch.Profile), batch.SubscriptionID, at))
		at = at.Add(delay)
	}
	holds.recordMetrics(batch, throttleReceipt{at: at})
	holds.recordMetrics(batch, throttleReceipt{status: 529, at: at})
	assert.Zero(t, holds.metricMask(batch, at.Add(time.Minute)), "successful retry reset the overload streak")
}

func testWireEvidenceBatch(names ...string) queryBatch {
	const namespace = "Microsoft.Cdn/profiles"
	id := "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.Cdn/profiles/edge"
	profile := &profileRuntime{Name: "front_door", MetricNamespace: namespace, namespaceKey: strings.ToLower(namespace)}
	var metrics []*metricRuntime
	for _, name := range names {
		metrics = append(metrics, &metricRuntime{AzureName: name, Aggregation: "total", Instrument: name})
	}
	return queryBatch{
		chunk:          &metricChunk{metrics: metrics},
		mask:           1<<len(metrics) - 1,
		SubscriptionID: "sub-1",
		Profile:        profile,
		MetricNames:    names,
		Aggregation:    "total",
		TimeGrain:      "PT1M",
		TimeGrainEvery: time.Minute,
		Region:         "eastus",
		Resources:      []resourceInfo{{ID: id, idKey: strings.ToLower(id), UID: "edge"}},
	}
}
