// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"golang.org/x/net/http2"
)

// azcore does not export its default client. Keep the pinned SDK's settings
// and share one connection pool for the two Azure APIs using this decorator.
var defaultAzureRequestClient = sync.OnceValue(func() *http.Client {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion:    tls.VersionTLS12,
			Renegotiation: tls.RenegotiateFreelyAsClient,
		},
	}
	if runtime.GOARCH != "wasm" {
		transport.DialContext = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
	if http2Transport, err := http2.ConfigureTransports(transport); err == nil {
		http2Transport.ReadIdleTimeout = 10 * time.Second
		http2Transport.PingTimeout = 5 * time.Second
	}
	return &http.Client{Transport: transport}
})

type azureRequestTransport struct {
	next policy.Transporter
}

type azureRoundTripper struct{ next http.RoundTripper }

func withAzureRequestTransport(options azcore.ClientOptions) azcore.ClientOptions {
	next := options.Transport
	if next == nil {
		next = defaultAzureRequestClient()
	}
	if client, ok := next.(*http.Client); ok {
		// Authorize each redirect after net/http prepares it. Retain the
		// client's redirect policy, timeouts and underlying connection pool.
		wrapped := *client
		transport := client.Transport
		if transport == nil {
			transport = http.DefaultTransport
		}
		wrapped.Transport = azureRoundTripper{next: transport}
		options.Transport = &wrapped
		return options
	}
	options.Transport = azureRequestTransport{next: next}
	return options
}

func (t azureRequestTransport) Do(req *http.Request) (*http.Response, error) {
	return doAzureRequest(req, t.next.Do)
}

func (t azureRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return doAzureRequest(req, t.next.RoundTrip)
}

func doAzureRequest(req *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	ctx := req.Context()
	op, _ := ctx.Value(queryOperationKey{}).(*queryOperation)
	if op == nil {
		return next(req)
	}
	post := req.Method == http.MethodPost
	if err := op.authorize(ctx, post); err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	if op.events != nil {
		// Keep the replay hook off the request http.Client uses for redirects.
		// Each redirect is authorized separately when it enters RoundTrip.
		rewind := req.GetBody
		if req.Method == http.MethodGet && (req.Body == nil || req.Body == http.NoBody) {
			// HTTP/1.1 and HTTP/2 can replay a transmitted GET internally.
			// They skip GetBody for nil and NoBody; an empty reader makes
			// them consult the same gate as a POST with a rewindable body.
			req = req.WithContext(ctx)
			req.Body = io.NopCloser(http.NoBody)
			rewind = func() (io.ReadCloser, error) { return io.NopCloser(http.NoBody), nil }
		} else if rewind != nil {
			req = req.WithContext(ctx)
		}
		if rewind != nil {
			req.GetBody = func() (io.ReadCloser, error) {
				body, err := rewind()
				if err != nil {
					return nil, err
				}
				if err := op.authorize(ctx, post); err != nil {
					_ = body.Close()
					return nil, err
				}
				return body, nil
			}
		}
	}
	resp, err := next(req)
	if resp != nil && throttleStatus(resp.StatusCode) {
		op.receive(resp)
	}
	return resp, err
}

type collectorClockKey struct{}

func withCollectorClock(ctx context.Context, now func() time.Time) context.Context {
	return context.WithValue(ctx, collectorClockKey{}, now)
}

type throttleReceipt struct {
	status  int
	headers http.Header
	at      time.Time
}

type receivedThrottleError struct {
	error
	receipt throttleReceipt
}

func (e *receivedThrottleError) Unwrap() error { return e.error }

// SDK authentication and retry can lose a throttle response when an error
// interrupts body download. Preserve the observed response and its receipt time.
func preserveCapturedThrottle(err error, receipt throttleReceipt) error {
	if receipt.status == 0 {
		return err
	}
	var response *azcore.ResponseError
	if !errors.As(err, &response) || !throttleStatus(response.StatusCode) {
		err = errors.Join(&azcore.ResponseError{
			StatusCode: receipt.status, ErrorCode: receipt.headers.Get("x-ms-error-code"),
			RawResponse: &http.Response{StatusCode: receipt.status, Header: receipt.headers},
		}, err)
	}
	return &receivedThrottleError{error: err, receipt: receipt}
}

type supplementalBudgetExhausted struct{}

var errSupplementalBudgetExhausted = supplementalBudgetExhausted{}

func (supplementalBudgetExhausted) Error() string {
	return "Azure Monitor supplemental Metrics request budget exhausted"
}

func (supplementalBudgetExhausted) NonRetriable() {}

type heldAzureRequest struct{}

var errHeldAzureRequest = heldAzureRequest{}

func (heldAzureRequest) Error() string { return "Azure Monitor request held by a service throttle" }

func (heldAzureRequest) NonRetriable() {}
