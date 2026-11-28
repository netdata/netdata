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
	return &http.Client{Transport: newAzureRequestHTTPTransport()}
})

func newAzureRequestHTTPTransport() *http.Transport {
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
	return transport
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
	_, native := t.next.(*http.Transport)
	return doAzureRequest(req, t.next.RoundTrip, native)
}

func doAzureRequest(req *http.Request, next func(*http.Request) (*http.Response, error), traceHTTP2 bool) (*http.Response, error) {
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
		req, cancel = gateAzureReplays(req, op, post, traceHTTP2)
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

func gateAzureReplays(req *http.Request, op *queryOperation, post, traceHTTP2 bool) (*http.Request, context.CancelCauseFunc) {
	ctx := req.Context()
	var cancel context.CancelCauseFunc
	var gotConn, h2, rewound, admitted bool
	if traceHTTP2 {
		ctx, cancel = context.WithCancelCause(ctx)
		// HTTP/2 obtains the next connection after the retry delay. GotConn
		// runs synchronously before headers, even if it reuses the same
		// connection. Its writer honors cancellation before sending headers.
		ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
			tlsConn, ok := info.Conn.(interface{ ConnectionState() tls.ConnectionState })
			h2 = ok && tlsConn.ConnectionState().NegotiatedProtocol == "h2"
			if gotConn && h2 && !admitted {
				if err := op.authorize(ctx, post); err != nil {
					cancel(err)
				}
			}
			gotConn, rewound, admitted = true, false, false
		}})
	}

	// Keep replay hooks off the request http.Client uses for redirects.
	// Each redirect is authorized separately when it enters RoundTrip.
	req = req.WithContext(ctx)
	rewind := req.GetBody
	if req.Method == http.MethodGet && (req.Body == nil || req.Body == http.NoBody) {
		// HTTP/1.1 skips GetBody for a bodyless GET. An empty reader makes
		// the transport consult the gate without adding a payload.
		req.Body = io.NopCloser(http.NoBody)
		rewind = func() (io.ReadCloser, error) { return io.NopCloser(http.NoBody), nil }
	}
	if rewind != nil {
		req.GetBody = func() (io.ReadCloser, error) {
			body, err := rewind()
			if err != nil {
				return nil, err
			}
			// HTTP/2 first rewinds before the retry delay. A second rewind
			// without GotConn is net/http falling back after HTTP/2 could
			// not get a connection. Admit it here as it can use HTTP/1.1.
			if !h2 || (rewound && !admitted) {
				if err := op.authorize(ctx, post); err != nil {
					_ = body.Close()
					return nil, err
				}
				admitted = true
			}
			rewound = true
			return body, nil
		}
	}
	return req, cancel
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
