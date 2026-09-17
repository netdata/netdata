// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"
)

type requestTraceKey struct{}
type requestTrace struct {
	stats       *wireStats
	first, last *responseData
	failure     error

	bootstrapUsername, bootstrapPassword string
}

// The SDK owns requests and authentication after bootstrap. This transport adds
// response timing/byte accounting and bounds reads before SDK error decoding.
type redfishTransport struct {
	base   http.RoundTripper
	root   *url.URL
	origin string
}

func (t *redfishTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if _, err := resolveRedfishURI(t.origin, t.root, req.URL.String(), uriOpaquePage); err != nil {
		return nil, err
	}
	trace, _ := req.Context().Value(requestTraceKey{}).(*requestTrace)
	if trace != nil && trace.bootstrapUsername != "" && req.Method == http.MethodGet &&
		req.Header.Get("Authorization") == "" {
		// Only Basic Connect carries these credentials, including root redirects.
		req = req.Clone(req.Context())
		req.SetBasicAuth(trace.bootstrapUsername, trace.bootstrapPassword)
	}
	if trace != nil && trace.last != nil && trace.last.status >= 200 && trace.last.status < 300 {
		trace.last.finish(nil)
	}
	started := time.Now()
	if trace != nil && trace.stats != nil {
		trace.stats.started++
	}
	response, err := t.base.RoundTrip(req)
	if err != nil {
		if trace != nil {
			trace.failure = sanitizeTransportError(err)
		}
		return nil, err
	}
	defer response.Body.Close()
	limit := int64(maxResponseBodyBytes)
	if response.StatusCode >= 400 {
		limit = maxErrorBodyBytes
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if trace != nil && trace.stats != nil {
		trace.stats.received += int64(len(body))
		trace.stats.unauthorized = trace.stats.unauthorized || response.StatusCode == http.StatusUnauthorized
	}
	if err != nil {
		err = sanitizeTransportError(err)
	} else if int64(len(body)) > limit {
		err = classifiedError{"limit", "Redfish response exceeds the internal body limit"}
	}
	if err == nil && req.Method == http.MethodPost && response.StatusCode >= 200 && response.StatusCode < 300 {
		// The SDK consumes these headers without validating them and strips the
		// origin from Location; validate before it loses that information.
		if response.Header.Get("X-Auth-Token") == "" {
			err = errors.New("Redfish session response has no authentication token")
		} else if _, locationErr := resolveRedfishURI(t.origin, req.URL, response.Header.Get("Location"), uriResource); locationErr != nil {
			err = errors.New("Redfish session response has an invalid Location")
		}
	}
	if trace != nil {
		trace.failure = err
		trace.last = &responseData{
			status:     response.StatusCode,
			body:       body,
			url:        req.URL,
			startedAt:  started,
			finishedAt: time.Now(),
			stats:      trace.stats,
		}
		if trace.first == nil && req.Method == http.MethodGet && response.StatusCode == http.StatusOK {
			trace.first = trace.last
		}
	}
	if err != nil {
		return nil, err
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	return response, nil
}

func (t *redfishTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func (t *redfishTransport) checkRedirect(req *http.Request, via []*http.Request) (err error) {
	defer func() {
		if trace, ok := req.Context().Value(requestTraceKey{}).(*requestTrace); ok && err != nil {
			trace.failure = err
		}
	}()
	previous := via[len(via)-1]
	if len(via) >= 10 {
		return errors.New("Redfish redirect limit exceeded")
	}
	if previous.Method != http.MethodGet && previous.Method != http.MethodHead {
		return errors.New("Redfish session request redirect is not allowed")
	}
	if _, err := resolveRedfishURI(t.origin, t.root, req.URL.String(), uriOpaquePage); err != nil {
		return err
	}
	if req.URL.RawQuery != previous.URL.RawQuery {
		return errors.New("Redfish redirect changed an authorized query")
	}
	if trace, ok := req.Context().Value(requestTraceKey{}).(*requestTrace); ok && trace.stats != nil {
		trace.stats.redirected++
	}
	return nil
}
