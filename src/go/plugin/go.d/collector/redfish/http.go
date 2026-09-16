// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func newHTTPClient(ctx context.Context, cfg Config) (*http.Client, error) {
	client, err := web.NewHTTPClient(ctx, web.ClientConfig{
		Timeout:           cfg.Timeout,
		NotFollowRedirect: true,
		ProxyURL:          cfg.ProxyURL,
		TLSConfig: tlscfg.TLSConfig{
			TLSCA:              cfg.TLSCA,
			TLSCert:            cfg.TLSCert,
			TLSKey:             cfg.TLSKey,
			InsecureSkipVerify: cfg.TLSSkipVerify,
		},
	})
	if err != nil {
		return nil, err
	}
	// Redfish applies its own exact-origin redirect policy.
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return client, nil
}

type protocolRequest struct {
	method string
	target *url.URL
	body   []byte
	auth   requestAuth
}

type statusError struct {
	status int
	class  string
	path   string
}

func (e statusError) Error() string {
	return fmt.Sprintf("Redfish request to %s returned HTTP %d", e.path, e.status)
}

type transportError struct {
	timeout   bool
	temporary bool
}

func (e transportError) Error() string {
	if e.timeout {
		return "Redfish transport timed out"
	}
	return "Redfish transport error"
}

func (e transportError) Timeout() bool { return e.timeout }

func (e transportError) Temporary() bool { return e.temporary }

func (c *protocolClient) do(
	ctx context.Context,
	request protocolRequest,
	stats *wireStats,
	retry bool,
	accepted ...int,
) (*responseData, error) {
	allowed := make(map[int]struct{}, len(accepted))
	for _, status := range accepted {
		allowed[status] = struct{}{}
	}
	retries := 0
	if retry && request.method == http.MethodGet && c.config.Retries != nil {
		retries = *c.config.Retries
	}
	sessionReplayed := false
	for attempt := 0; ; {
		response, err := c.doRedirectChain(ctx, request, stats)
		if err == nil {
			if _, ok := allowed[response.status]; ok {
				return response, nil
			}
			if response.status == http.StatusUnauthorized && request.method == http.MethodGet &&
				request.auth.mode == "session" && !sessionReplayed {
				replacement, refreshErr := c.refreshSession(ctx, request.auth.token, stats)
				if refreshErr != nil {
					recordWireFailure(stats, "auth")
					return nil, refreshErr
				}
				sessionReplayed = true
				request.auth = replacement
				if stats != nil {
					stats.retried++
				}
				continue
			}
			err = statusError{
				status: response.status,
				class:  classifyHTTPStatus(response.status),
				path:   response.url.EscapedPath(),
			}
			if !retryableStatus(response.status) {
				recordWireFailure(stats, classifyHTTPStatus(response.status))
				return nil, err
			}
		} else if !retryableTransport(err) {
			recordWireFailure(stats, classifyError(err))
			return nil, err
		}
		if attempt >= retries {
			recordWireFailure(stats, classifyError(err))
			return nil, err
		}
		delay := retryDelay(attempt, response)
		if delay > 0 {
			if sleepErr := sleepContext(ctx, delay); sleepErr != nil {
				recordWireFailure(stats, classifyError(sleepErr))
				return nil, sleepErr
			}
		}
		attempt++
		if stats != nil {
			stats.retried++
		}
	}
}

func recordWireFailure(stats *wireStats, class string) {
	if stats == nil {
		return
	}
	stats.failed++
	if stats.failures == nil {
		stats.failures = make(map[string]int)
	}
	stats.failures[class]++
}

func retryDelay(attempt int, response *responseData) time.Duration {
	if response != nil {
		if delay := retryAfter(response.header); delay > 0 {
			return delay
		}
	}
	const initial = 100 * time.Millisecond
	delay := initial << min(attempt, 4)
	return min(delay, time.Second)
}

func (c *protocolClient) doRedirectChain(
	ctx context.Context,
	request protocolRequest,
	stats *wireStats,
) (*responseData, error) {
	seen := make(map[string]struct{}, maxRedirects+1)
	for redirects := 0; ; redirects++ {
		key := request.target.String()
		if _, ok := seen[key]; ok {
			return nil, errors.New("Redfish redirect loop")
		}
		seen[key] = struct{}{}
		response, err := c.doOnce(ctx, request, stats)
		if err != nil {
			return nil, err
		}
		if response.status < 300 || response.status > 399 {
			return response, nil
		}
		if redirects >= maxRedirects {
			return nil, classifiedError{"limit", "Redfish redirect limit exceeded"}
		}
		if request.method != http.MethodGet && request.method != http.MethodHead {
			return nil, errors.New("Redfish session request redirect is not allowed")
		}
		location := response.header.Get("Location")
		if location == "" {
			return nil, errors.New("Redfish redirect has no Location")
		}
		next, err := c.resolveURI(request.target, location, request.target.RawQuery != "")
		if err != nil {
			return nil, fmt.Errorf("reject Redfish redirect: %w", err)
		}
		if next.RawQuery != request.target.RawQuery {
			return nil, errors.New("Redfish redirect changed an authorized query")
		}
		if stats != nil {
			stats.redirected++
		}
		request.target = next
	}
}

func (c *protocolClient) doOnce(
	ctx context.Context,
	spec protocolRequest,
	stats *wireStats,
) (*responseData, error) {
	sem, err := c.acquireRequest(ctx)
	if err != nil {
		return nil, err
	}
	defer releaseRequest(sem)

	var reader io.Reader
	if len(spec.body) > 0 {
		reader = bytes.NewReader(spec.body)
	}
	request, err := http.NewRequestWithContext(ctx, spec.method, spec.target.String(), reader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("OData-Version", "4.0")
	if len(spec.body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	switch spec.auth.mode {
	case "basic":
		request.SetBasicAuth(spec.auth.username, spec.auth.password)
	case "session":
		if spec.auth.token == "" {
			return nil, errors.New("Redfish session token is unavailable")
		}
		request.Header.Set("X-Auth-Token", spec.auth.token)
	}
	if spec.auth.token != "" && spec.auth.mode == "" {
		request.Header.Set("X-Auth-Token", spec.auth.token)
	}
	if stats != nil {
		stats.started++
	}
	startedAt := time.Now()
	response, err := c.http.Do(request)
	if err != nil {
		return nil, sanitizeTransportError(err)
	}
	defer response.Body.Close()
	limit := int64(maxResponseBodyBytes)
	if response.StatusCode >= 400 {
		limit = maxErrorBodyBytes
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	finishedAt := time.Now()
	if stats != nil {
		stats.received += int64(len(payload))
	}
	if err != nil {
		return nil, sanitizeTransportError(err)
	}
	if int64(len(payload)) > limit {
		return nil, classifiedError{"limit", "Redfish response exceeds the internal body limit"}
	}
	return &responseData{
		status:     response.StatusCode,
		header:     response.Header.Clone(),
		body:       payload,
		url:        response.Request.URL,
		startedAt:  startedAt,
		finishedAt: finishedAt,
		stats:      stats,
	}, nil
}

func (c *protocolClient) setRequestLimit(limit int) {
	if limit < 1 {
		limit = 1
	}
	c.semMu.Lock()
	if cap(c.sem) != limit {
		c.sem = make(chan struct{}, limit)
	}
	c.semMu.Unlock()
}

func (c *protocolClient) acquireRequest(ctx context.Context) (chan struct{}, error) {
	c.semMu.RLock()
	sem := c.sem
	c.semMu.RUnlock()
	select {
	case sem <- struct{}{}:
		return sem, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func releaseRequest(sem chan struct{}) {
	<-sem
}

func retryableStatus(status int) bool {
	return status == http.StatusRequestTimeout ||
		status == http.StatusTooManyRequests ||
		status >= 500
}

func retryAfter(header http.Header) time.Duration {
	value := header.Get("Retry-After")
	if len(value) > maxRetryAfterBytes {
		return 0
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	seconds, err := strconv.ParseUint(value, 10, 64)
	if err == nil && seconds > 0 {
		maxSeconds := uint64(maxRetryAfter / time.Second)
		if seconds >= maxSeconds {
			return maxRetryAfter
		}
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		return min(max(time.Until(when), 0), maxRetryAfter)
	}
	return 0
}

func retryableTransport(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Temporary() ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE)
}

func sanitizeTransportError(err error) error {
	if err == nil {
		return nil
	}
	var tlsErr tls.RecordHeaderError
	if errors.As(err, &tlsErr) {
		return classifiedError{"tls", "Redfish TLS protocol error"}
	}
	var verificationError *tls.CertificateVerificationError
	var unknownAuthority x509.UnknownAuthorityError
	var hostnameError x509.HostnameError
	if errors.As(err, &verificationError) ||
		errors.As(err, &unknownAuthority) ||
		errors.As(err, &hostnameError) {
		return classifiedError{"tls", "Redfish TLS certificate verification failed"}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return transportError{
			timeout: netErr.Timeout(),
			temporary: netErr.Temporary() ||
				errors.Is(err, syscall.ECONNRESET) ||
				errors.Is(err, syscall.EPIPE),
		}
	}
	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return transportError{
			temporary: true,
		}
	}
	return transportError{}
}

func classifyHTTPStatus(status int) string {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "auth"
	case http.StatusRequestTimeout, http.StatusTooManyRequests:
		return "timeout"
	default:
		return "protocol"
	}
}

func classifyError(err error) string {
	var status statusError
	if errors.As(err, &status) {
		return status.class
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	var classified classifiedError
	if errors.As(err, &classified) {
		return classified.class
	}
	var tlsErr tls.RecordHeaderError
	if errors.As(err, &tlsErr) {
		return "tls"
	}
	if errors.As(err, &netErr) {
		return "transport"
	}
	return "protocol"
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Classified errors retain a safe message without deriving policy from its wording.
type classifiedError struct{ class, message string }

func (e classifiedError) Error() string { return e.message }

const (
	maxResponseBodyBytes = 16 << 20
	maxErrorBodyBytes    = 64 << 10
	maxRedirects         = 3
	maxRetryAfter        = 5 * time.Second
	maxRetryAfterBytes   = 128
)
