// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const downloadIdleTimeout = 60 * time.Second

type inactivityError struct{}

func (*inactivityError) Error() string   { return "runtime download network inactivity timeout" }
func (*inactivityError) Timeout() bool   { return true }
func (*inactivityError) Temporary() bool { return true }

var errDownloadInactive = &inactivityError{}

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
		// Do not expose the internal cancellation as voluntary cancellation:
		// the host must retain its normal acquisition-failure retry backoff.
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

// Downloads are explicit candidate operations, never monitor lifecycle work.
func (s *Store) download(ctx context.Context, m Manifest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if m.Archive.URL == "" {
		return errors.New("runtime manifest has no download source")
	}
	return s.prepare(ctx, m, func() (io.ReadCloser, error) {
		return openDownload(ctx, m.Archive.URL, downloadIdleTimeout)
	})
}
