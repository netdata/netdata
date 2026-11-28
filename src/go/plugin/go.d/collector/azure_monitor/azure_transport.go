// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
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
	return &http.Client{Transport: newAzureRequestHTTPTransport(http.ProxyFromEnvironment)}
})

type azureNativeTransport struct{ *http.Transport }

func newAzureRequestHTTPTransport(proxy func(*http.Request) (*url.URL, error)) *azureNativeTransport {
	transport := &http.Transport{
		// net/http resolves the proxy on each outer attempt, before it takes
		// a connection. Returning an admission error here prevents an HTTP/1
		// write even when a stale pooled connection needed no body rewind.
		Proxy: func(req *http.Request) (*url.URL, error) {
			if a, ok := req.Context().Value(nativeAttemptKey{}).(*nativeAttempt); ok {
				if err := a.beforeConnect(req.Context()); err != nil {
					return nil, err
				}
			}
			if proxy != nil {
				return proxy(req)
			}
			return nil, nil
		},
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
	return &azureNativeTransport{transport}
}

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
		if _, owned := client.Transport.(*azureNativeTransport); owned {
			options.Transport = client
			return options
		}
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
	return doAzureRequest(req, t.next.Do, false)
}

func (t azureRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return doAzureRequest(req, t.next.RoundTrip, false)
}

func (t *azureNativeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return doAzureRequest(req, t.Transport.RoundTrip, true)
}

func doAzureRequest(req *http.Request, next func(*http.Request) (*http.Response, error), native bool) (*http.Response, error) {
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
	var cancel context.CancelCauseFunc
	if op.events != nil {
		if native {
			req, cancel = trackNativeAttempts(req, op, post)
		} else {
			req = gateTransportRewinds(req, op, post)
		}
	}
	resp, err := next(req)
	if cancel != nil {
		if cause := context.Cause(req.Context()); cause != nil && err != nil {
			err = cause
		}
		if resp != nil && resp.Body != nil {
			resp.Body = azureResponseBody{ReadCloser: resp.Body, cancel: cancel}
		} else {
			cancel(nil)
		}
	}
	if resp != nil && throttleStatus(resp.StatusCode) {
		op.receive(resp)
	}
	return resp, err
}

type nativeAttemptKey struct{}

// net/http can retry in its outer connection loop or inside a single HTTP/2
// RoundTrip. The first connection selected in each outer attempt shares its
// admission. A second selection is an HTTP/2 retry and needs a fresh decision.
type nativeAttempt struct {
	op       *queryOperation
	post     bool
	started  bool // The initial admission already covers the first acquisition.
	selected bool // GotConn ran since the most recent outer acquisition.
	rewound  bool // net/http had to reconstruct the preceding request body.
}

func (a *nativeAttempt) beforeConnect(ctx context.Context) error {
	if a.started {
		// An HTTP/1 Metrics POST retried with its untouched body wrote zero
		// bytes. Check holds again, reusing its existing POST reservation.
		if err := a.op.authorize(ctx, a.post && a.selected && a.rewound); err != nil {
			return err
		}
	}
	a.started, a.selected, a.rewound = true, false, false
	return nil
}

func trackNativeAttempts(req *http.Request, op *queryOperation, post bool) (*http.Request, context.CancelCauseFunc) {
	a := &nativeAttempt{op: op, post: post}
	ctx, cancel := context.WithCancelCause(context.WithValue(req.Context(), nativeAttemptKey{}, a))
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{GotConn: func(httptrace.GotConnInfo) {
		if a.selected {
			// Only the inner HTTP/2 loop selects twice without resolving
			// a proxy. It checks cancellation after backoff, before HEADERS.
			if err := op.authorize(ctx, post); err != nil {
				cancel(err)
			}
		}
		a.started, a.selected, a.rewound = true, true, false
	}})
	req = req.WithContext(ctx)
	if rewind := req.GetBody; post && rewind != nil {
		req.GetBody = func() (io.ReadCloser, error) {
			body, err := rewind()
			a.rewound = err == nil
			return body, err
		}
	}
	return req, cancel
}

// Injected transports expose entry and body reconstruction. Leave their
// connection pool and redirect policy under their owner's control.
func gateTransportRewinds(req *http.Request, op *queryOperation, post bool) *http.Request {
	rewind := req.GetBody
	if req.Method == http.MethodGet && (req.Body == nil || req.Body == http.NoBody) {
		req = req.WithContext(req.Context())
		req.Body = io.NopCloser(http.NoBody)
		rewind = func() (io.ReadCloser, error) { return io.NopCloser(http.NoBody), nil }
	} else if rewind != nil {
		req = req.WithContext(req.Context())
	}
	if rewind != nil {
		req.GetBody = func() (io.ReadCloser, error) {
			body, err := rewind()
			if err == nil {
				if err := op.authorize(req.Context(), post); err != nil {
					_ = body.Close()
					return nil, err
				}
			}
			return body, err
		}
	}
	return req
}

type azureResponseBody struct {
	io.ReadCloser
	cancel context.CancelCauseFunc
}

func (b azureResponseBody) Close() error {
	defer b.cancel(nil)
	return b.ReadCloser.Close()
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
