// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTP1MetricsRetryAfterZeroWrite(t *testing.T) {
	for _, tc := range []struct {
		name            string
		held, failFlush bool
	}{
		{name: "untouched body uses original reservation"},
		{name: "untouched body checks hold", held: true},
		{name: "buffered body uses original reservation", failFlush: true},
		{name: "buffered body checks hold", failFlush: true, held: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				ctx = policy.WithRetryOptions(ctx, policy.RetryOptions{MaxRetries: -1})
				client, reset, firstConn, requests, payloads := http1ReplayClient(t, ctx, tc.held, tc.failFlush)
				p, launch, first, other := delayedReplayOperations(t, client, true)
				p.supplementalUsed = maxRecoveryMetricsCalls // Includes the selected request's dispatch reservation.
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

				selected := make(chan httptrace.GotConnInfo, 1)
				resume := make(chan struct{})
				reqCtx := httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
					if info.Conn == firstConn {
						selected <- info
						select {
						case <-resume:
						case <-ctx.Done():
						}
					}
				}})
				done := launch(reqCtx, first)
				select {
				case info := <-selected:
					require.True(t, info.Reused)
				case <-ctx.Done():
					t.Fatal("the Metrics request did not select the warmed connection")
				}
				if tc.held {
					var response *azcore.ResponseError
					require.ErrorAs(t, <-launch(ctx, other), &response)
					require.Equal(t, http.StatusTooManyRequests, response.StatusCode)
					require.Equal(t, http.MethodPost, <-requests)
					<-payloads
				}

				if !tc.failFlush {
					reset()
					<-firstConn.closed
					// Let net/http close the writer before it takes the body.
					synctest.Wait()
				}
				require.EqualValues(t, 1, firstConn.writes.Load(), "only warm-up wrote to the first connection")
				close(resume)
				err := <-done
				synctest.Wait()
				wantWrites := 1 // The warm-up request.
				if tc.failFlush {
					wantWrites++ // The attempted flush wrote zero bytes.
				}
				assert.EqualValues(t, wantWrites, firstConn.writes.Load())
				assert.Equal(t, maxRecoveryMetricsCalls, p.supplementalUsed)
				if tc.held {
					assert.ErrorIs(t, err, errHeldAzureRequest)
					assert.Zero(t, len(requests), "the held POST must send no headers")
					assert.Zero(t, len(payloads))
					return
				}
				require.NoError(t, err)
				require.Equal(t, 1, len(requests))
				require.Equal(t, 1, len(payloads))
				assert.Equal(t, http.MethodPost, <-requests)
				assert.JSONEq(t, `{"resourceids":["`+first.request.batch.Resources[0].ID+`"]}`, <-payloads)
			})
		})
	}
}

func http1ReplayClient(t *testing.T, ctx context.Context, held, failFlush bool) (*http.Client, func(), *resetBeforeWriteConn, <-chan string, <-chan string) {
	t.Helper()
	transport := newAzureRequestHTTPTransport(nil)
	var dials atomic.Int32
	var peers sync.WaitGroup
	initial := make(chan *resetBeforeWriteConn, 1)
	resetPeer := make(chan struct{})
	requests, payloads := make(chan string, 4), make(chan string, 4)
	transport.DialTLSContext = func(context.Context, string, string) (net.Conn, error) {
		client, peer := net.Pipe()
		deadline, _ := ctx.Deadline()
		_ = peer.SetDeadline(deadline)
		dial := dials.Add(1)
		peers.Go(func() {
			defer peer.Close()
			r := bufio.NewReader(peer)
			for attempt := 1; ; attempt++ {
				req, err := http.ReadRequest(r)
				if err != nil {
					return
				}
				if req.URL.Path == "/warmup" {
					_ = req.Body.Close()
					_, _ = io.WriteString(peer, "HTTP/1.1 204 No Content\r\n\r\n")
					if failFlush {
						continue
					}
					select {
					case <-resetPeer:
					case <-ctx.Done():
					}
					return
				}
				requests <- req.Method
				body, err := io.ReadAll(req.Body)
				_ = req.Body.Close()
				if err != nil {
					return
				}
				payloads <- string(body)
				if held && dial == 2 && attempt == 1 {
					const throttled = `{"error":{"code":"TooManyRequests"}}`
					_, _ = fmt.Fprintf(peer, "HTTP/1.1 429 Too Many Requests\r\nRetry-After: 60\r\nContent-Length: %d\r\n\r\n%s", len(throttled), throttled)
					continue
				}
				_, _ = fmt.Fprintf(peer, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", len(emptyMetricDefinitions), emptyMetricDefinitions)
			}
		})
		if dial != 1 {
			return client, nil
		}
		fault := &resetBeforeWriteConn{Conn: client, closed: make(chan struct{}), failFlush: failFlush}
		initial <- fault
		return fault, nil
	}
	t.Cleanup(func() {
		transport.CloseIdleConnections()
		peers.Wait()
	})
	client := &http.Client{Transport: transport}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://eastus.metrics.monitor.azure.com/warmup", nil)
	require.NoError(t, err)
	warmed, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, warmed.Body.Close())
	first := <-initial
	reset := func() {
		first.reset.Store(true)
		close(resetPeer)
	}
	return client, reset, first, requests, payloads
}

type resetBeforeWriteConn struct {
	net.Conn
	failFlush bool
	reset     atomic.Bool
	writes    atomic.Int32
	closed    chan struct{}
	once      sync.Once
}

func (c *resetBeforeWriteConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if err != nil && c.reset.Load() {
		// A peer reset lets the native transport retry a zero-byte POST.
		return n, syscall.ECONNRESET
	}
	return n, err
}

func (c *resetBeforeWriteConn) Write(b []byte) (int, error) {
	if count := c.writes.Add(1); c.failFlush && count == 2 {
		return 0, syscall.ECONNRESET
	}
	return c.Conn.Write(b)
}

func (c *resetBeforeWriteConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { close(c.closed) })
	return err
}
