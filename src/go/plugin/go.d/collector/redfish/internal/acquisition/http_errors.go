// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
)

type statusError struct {
	status      int
	class, path string
}

func (e statusError) Error() string {
	return fmt.Sprintf("Redfish request to %s returned HTTP %d", e.path, e.status)
}

type transportError struct{ timeout bool }

func (e transportError) Error() string {
	if e.timeout {
		return "Redfish transport timed out"
	}
	return "Redfish transport error"
}
func (e transportError) Timeout() bool   { return e.timeout }
func (e transportError) Temporary() bool { return false }

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

type classifiedError struct{ class, message string }

func (e classifiedError) Error() string { return e.message }

const (
	maxResponseBodyBytes = 16 << 20
	maxErrorBodyBytes    = 64 << 10
)

func isCallerContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
