// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"syscall"
	"time"
)

const downloadIdleTimeout = 60 * time.Second

var errDownloadInactive = errors.New("runtime download network inactivity timeout")

// downloadActivity owns one HTTP request chain. Its timer only measures time
// waiting for the network, never archive writes or other local preparation work.
type downloadActivity struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	idle   time.Duration
	mu     sync.Mutex
	timer  *time.Timer
	epoch  uint64
	closed bool
}

func (a *downloadActivity) arm() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return
	}
	a.stopLocked()
	epoch := a.epoch
	a.timer = time.AfterFunc(a.idle, func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if !a.closed && a.epoch == epoch {
			a.cancel(errDownloadInactive)
		}
	})
}

func (a *downloadActivity) stopLocked() {
	a.epoch++
	if a.timer != nil {
		a.timer.Stop()
		a.timer = nil
	}
}

func (a *downloadActivity) disarm() {
	a.mu.Lock()
	a.stopLocked()
	a.mu.Unlock()
}

func (a *downloadActivity) close() {
	a.mu.Lock()
	a.closed = true
	a.stopLocked()
	a.cancel(context.Canceled)
	a.mu.Unlock()
}

func (a *downloadActivity) failure(err error) error {
	if errors.Is(context.Cause(a.ctx), errDownloadInactive) {
		// Report inactivity as a download failure, never as context
		// cancellation: callers treat cancellation as their own decision.
		return &downloadError{cause: errDownloadInactive, detail: "network inactivity timeout"}
	}
	if err == nil || err == io.EOF {
		return err
	}
	return downloadFailure(err)
}

type downloadBody struct {
	io.ReadCloser
	activity *downloadActivity
}

func (b *downloadBody) Read(p []byte) (int, error) {
	b.activity.arm()
	n, err := b.ReadCloser.Read(p)
	b.activity.disarm()
	return n, b.activity.failure(err)
}

func (b *downloadBody) Close() error {
	b.activity.close()
	return b.ReadCloser.Close()
}

func openDownload(ctx context.Context, address string, idle time.Duration) (io.ReadCloser, error) {
	requestCtx, cancel := context.WithCancelCause(ctx)
	activity := &downloadActivity{ctx: requestCtx, cancel: cancel, idle: idle}
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, address, nil)
	if err != nil {
		activity.close()
		return nil, errors.New("invalid runtime download request")
	}
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 || req.URL.Scheme != "https" || req.URL.User != nil {
			return errDownloadRedirect
		}
		activity.arm()
		return nil
	}}
	activity.arm()
	response, err := client.Do(request)
	activity.disarm()
	if err = activity.failure(err); err != nil {
		activity.close()
		if response != nil {
			response.Body.Close()
		}
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		activity.close()
		response.Body.Close()
		return nil, fmt.Errorf("runtime download returned HTTP %d", response.StatusCode)
	}
	return &downloadBody{ReadCloser: response.Body, activity: activity}, nil
}

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
