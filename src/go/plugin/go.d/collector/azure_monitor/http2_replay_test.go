// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"net"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2/hpack"
)

func TestHTTP2DelayedReplayAdmission(t *testing.T) {
	for _, tc := range []struct {
		name    string
		post    bool
		hold    bool
		wantErr error
	}{
		{name: "ARM replay allowed"},
		{name: "ARM replay held", hold: true, wantErr: errHeldAzureRequest},
		{name: "ARM deadline", wantErr: context.DeadlineExceeded},
		{name: "Metrics replay allowed", post: true},
		{name: "Metrics replay held", post: true, hold: true, wantErr: errHeldAzureRequest},
		{name: "Metrics replay exceeds budget", post: true, wantErr: errSupplementalBudgetExhausted},
		{name: "Metrics deadline", post: true, wantErr: context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				wire := make(chan wireDefinitionRead, 4)
				headers := make(chan struct{}, 4)
				client := replayHTTP2Client(t, ctx, func(peer net.Conn) {
					attempts := 0
					serveAzureReplayHTTP2(peer, func() { headers <- struct{}{} }, func(req wireDefinitionRead) *wireReplayResponse {
						attempts++
						wire <- req
						if attempts <= 2 {
							return &wireReplayResponse{} // Only the second replay waits before sending.
						}
						if attempts == 3 && tc.hold {
							return &wireReplayResponse{
								status: http.StatusTooManyRequests,
								headers: []hpack.HeaderField{
									{Name: "retry-after", Value: "60"},
									{Name: "x-ms-ratelimit-remaining-subscription-reads", Value: "0"},
								},
								body: `{"error":{"code":"TooManyRequests"}}`,
							}
						}
						return &wireReplayResponse{status: http.StatusOK, body: emptyMetricDefinitions}
					})
				})
				ctx = policy.WithRetryOptions(ctx, policy.RetryOptions{MaxRetries: -1})
				p, launch, first, other := delayedReplayOperations(t, client, tc.post)
				if tc.wantErr == errSupplementalBudgetExhausted {
					p.supplementalUsed++ // Only the immediate replay can still fit.
				}
				go func() {
					for {
						select {
						case e := <-p.events:
							p.onEvent(e)
						case <-ctx.Done():
							return
						}
					}
				}()

				reqCtx := ctx
				if tc.wantErr == context.DeadlineExceeded {
					var expire context.CancelFunc
					reqCtx, expire = context.WithTimeout(ctx, 500*time.Millisecond)
					defer expire()
				}
				result := launch(reqCtx, first)
				var initial wireDefinitionRead
				select {
				case initial = <-wire:
				case <-ctx.Done():
					t.Fatal("the HTTP/2 peer did not receive the initial request")
				}
				select {
				case immediate := <-wire:
					require.Equal(t, initial, immediate)
				case <-ctx.Done():
					t.Fatal("the HTTP/2 peer did not receive the immediate replay")
				}
				synctest.Wait()
				require.Equal(t, start, time.Now(), "both refusals finish before the backoff")
				if tc.hold {
					var throttled *azcore.ResponseError
					require.ErrorAs(t, <-launch(ctx, other), &throttled)
					require.Equal(t, http.StatusTooManyRequests, throttled.StatusCode)
					<-wire
					require.Equal(t, start, time.Now(), "the 429 must precede the delayed replay")
				}

				err := <-result
				synctest.Wait()
				wantHeaders := 2
				if tc.hold {
					wantHeaders++
				}
				if tc.wantErr == nil {
					assert.NoError(t, err)
					require.Equal(t, 1, len(wire))
					assert.Equal(t, initial, <-wire)
					wantHeaders++
				} else {
					assert.ErrorIs(t, err, tc.wantErr)
					assert.Zero(t, len(wire))
				}
				assert.Equal(t, wantHeaders, len(headers), "count requests even if no body was sent")
				if tc.wantErr == context.DeadlineExceeded {
					assert.Equal(t, 500*time.Millisecond, time.Since(start))
				} else if tc.wantErr != errSupplementalBudgetExhausted {
					assert.GreaterOrEqual(t, time.Since(start), time.Second)
					assert.LessOrEqual(t, time.Since(start), 1100*time.Millisecond)
				}
				if tc.post {
					assert.Equal(t, http.MethodPost, initial.method)
					assert.JSONEq(t, `{"resourceids":["`+first.request.batch.Resources[0].ID+`"]}`, initial.body)
					wantBudget := maxRecoveryMetricsCalls
					if tc.hold || tc.wantErr == context.DeadlineExceeded {
						wantBudget--
					}
					assert.Equal(t, wantBudget, p.supplementalUsed)
				}
			})
		})
	}
}

func delayedReplayOperations(t *testing.T, client *http.Client, post bool) (*queryPoll, func(context.Context, *queryOperation) <-chan error, *queryOperation, *queryOperation) {
	t.Helper()
	batch := testWireEvidenceBatch("A")
	p := &queryPoll{
		c:      &Collector{now: time.Now},
		state:  &recoveryState{},
		events: make(chan queryEvent, 2),
	}
	first, other := p.takeOperation(), p.takeOperation()
	cred := new(fake.TokenCredential)
	opts := withAzureRequestTransport(azcore.ClientOptions{Transport: client})
	var send func(context.Context, *queryOperation) error
	if post {
		p.supplementalUsed = maxRecoveryMetricsCalls - 2 // Includes first dispatch's reservation.
		first.request = plannedRequest{batch: batch, questions: []*metricQuestion{{batch: batch}}}
		other.request = plannedRequest{batch: batch}
		metrics, err := azmetrics.NewClient("https://eastus.metrics.monitor.azure.com", cred, &azmetrics.ClientOptions{ClientOptions: opts})
		require.NoError(t, err)
		send = func(ctx context.Context, _ *queryOperation) error {
			_, err := metrics.QueryResources(ctx, batch.SubscriptionID, batch.Profile.MetricNamespace, batch.MetricNames,
				azmetrics.ResourceIDList{ResourceIDs: []string{batch.Resources[0].ID}}, nil)
			return err
		}
	} else {
		key := definitionKey{resourceID: batch.Resources[0].ID, namespace: batch.Profile.MetricNamespace}
		otherKey := definitionKey{resourceID: key.resourceID + "-other", namespace: key.namespace}
		first.read = &adviceRequest{key: key.canonical(), subscription: batch.SubscriptionID}
		other.read = &adviceRequest{key: otherKey.canonical(), subscription: batch.SubscriptionID}
		p.state.advice = map[definitionKey]*definitionAdvice{first.read.key: {}, other.read.key: {}}
		armClient, err := arm.NewClient("test", "v1.0.0", cred, &arm.ClientOptions{ClientOptions: opts})
		require.NoError(t, err)
		definitions := &metricDefinitionsClient{armClient}
		send = func(ctx context.Context, op *queryOperation) error {
			_, err := definitions.list(ctx, op.read.key)
			return err
		}
	}
	launch := func(ctx context.Context, op *queryOperation) <-chan error {
		done := make(chan error, 1)
		go func() { done <- send(context.WithValue(ctx, queryOperationKey{}, op), op) }()
		return done
	}
	return p, launch, first, other
}
