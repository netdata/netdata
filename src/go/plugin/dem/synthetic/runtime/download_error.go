// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"syscall"
)

var errDownloadRedirect = errors.New("unsafe runtime download redirect")

// HTTP parser errors can contain raw redirect locations, including credentials.
// Keep the cause for classification without displaying untrusted error text.
type downloadError struct {
	cause  error
	detail string
}

func (e *downloadError) Error() string { return "runtime download failed: " + e.detail }
func (e *downloadError) Unwrap() error { return e.cause }

func downloadFailure(err error) error {
	detail := "HTTP request failed"
	var dns *net.DNSError
	var certificate *tls.CertificateVerificationError
	var record tls.RecordHeaderError
	var network net.Error
	var errno syscall.Errno
	switch {
	case errors.Is(err, context.Canceled):
		detail = "context canceled"
	case errors.Is(err, context.DeadlineExceeded):
		detail = "request deadline exceeded"
	case errors.Is(err, errDownloadRedirect):
		detail = errDownloadRedirect.Error()
	case errors.As(err, &dns):
		detail = "DNS lookup failed"
		if dns.IsNotFound {
			detail = "DNS name not found"
		} else if dns.IsTimeout {
			detail = "DNS lookup timed out"
		}
	case errors.As(err, &certificate):
		detail = "TLS certificate verification failed"
	case errors.As(err, &record):
		detail = "TLS handshake failed"
	case errors.As(err, &network) && network.Timeout():
		detail = "network operation timed out"
	case errors.As(err, &errno):
		detail = errno.Error()
	}
	return &downloadError{
		cause:  err,
		detail: detail,
	}
}
