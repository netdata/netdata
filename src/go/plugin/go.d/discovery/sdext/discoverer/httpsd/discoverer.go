// SPDX-License-Identifier: GPL-3.0-or-later

package httpsd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/safefile"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

const (
	shortName = "http"
	fullName  = "sd:http"

	publicErrRequest = "the configured HTTP discovery request could not be created"
	publicErrTimeout = "the configured HTTP endpoint did not respond before the timeout"
	publicErrQuery   = "cannot query the configured HTTP endpoint"
	publicErrData    = "the configured HTTP endpoint did not return usable discovery data"
	publicErrFile    = "the configured HTTP credential or TLS file could not be read safely"
)

func NewDiscoverer(cfg Config) (*Discoverer, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	client, err := web.NewHTTPClient(cfg.clientConfig())
	if err != nil {
		if errors.Is(err, tlscfg.ErrTLSFile) || errors.Is(err, safefile.ErrFile) {
			return nil, dyncfg.NewPublicError(publicErrFile, err)
		}
		return nil, err
	}

	d := &Discoverer{
		Logger: logger.New().With(
			slog.String("component", "service discovery"),
			slog.String("discoverer", shortName),
		),
		client:   client,
		request:  cfg.RequestConfig,
		interval: cfg.interval(),
		parser:   responseParser{format: cfg.format()},
		source:   sourceString(cfg),
	}

	return d, nil
}

type Discoverer struct {
	*logger.Logger
	model.Base

	client  *http.Client
	request web.RequestConfig

	interval time.Duration
	parser   responseParser
	source   string
}

func (d *Discoverer) String() string {
	return fullName
}

func (d *Discoverer) Test(ctx context.Context) error {
	if d == nil || ctx == nil {
		return errors.New("invalid HTTP discovery test")
	}
	if d.request.Method != "" && d.request.Method != http.MethodGet {
		return dyncfg.ErrTestUnsupported
	}

	defer d.client.CloseIdleConnections()

	_, err := d.fetchTargetGroup(ctx)
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, safefile.ErrFile), errors.Is(err, tlscfg.ErrTLSFile):
		return dyncfg.NewPublicError(publicErrFile, err)
	case errors.Is(err, context.DeadlineExceeded):
		return dyncfg.NewPublicError(publicErrTimeout, err)
	}

	var fetchErr *fetchError
	if errors.As(err, &fetchErr) {
		switch fetchErr.phase {
		case fetchPhaseRequest:
			return dyncfg.NewPublicError(publicErrRequest, err)
		case fetchPhaseQuery:
			return dyncfg.NewPublicError(publicErrQuery, err)
		case fetchPhaseResponse:
			return dyncfg.NewPublicError(publicErrData, err)
		}
	}
	return dyncfg.NewPublicError(publicErrQuery, err)
}

func (d *Discoverer) Discover(ctx context.Context, in chan<- []model.TargetGroup) {
	d.Info("instance is started")
	d.Debugf("used config: interval: %s, response body limit: %d, source: %s", d.interval, responseBodyLimit, d.source)
	defer func() { d.Info("instance is stopped") }()

	d.discover(ctx, in)

	if d.interval <= 0 {
		return
	}

	tk := time.NewTicker(d.interval)
	defer tk.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			d.discover(ctx, in)
		}
	}
}

func (d *Discoverer) discover(ctx context.Context, in chan<- []model.TargetGroup) {
	tgg, err := d.fetchTargetGroup(ctx)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			d.Warning(err)
		}
		return
	}

	model.SendTargetGroup(ctx, in, tgg)
}

func (d *Discoverer) fetchTargetGroup(ctx context.Context) (model.TargetGroup, error) {
	req, err := web.NewHTTPRequest(d.request)
	if err != nil {
		return nil, newFetchError(fetchPhaseRequest, fmt.Errorf("create HTTP request: %w", err))
	}
	req = req.WithContext(ctx)
	safeURL := sanitizedURL(req.URL.String())

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, newFetchError(fetchPhaseQuery, fmt.Errorf("HTTP request to %q failed: %w", safeURL, err))
	}
	if resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}

	if resp.StatusCode != http.StatusOK {
		return nil, newFetchError(
			fetchPhaseQuery,
			fmt.Errorf("%s %q returned HTTP status code: %d", req.Method, safeURL, resp.StatusCode),
		)
	}

	bs, err := readResponseBody(resp.Body, responseBodyLimit)
	if err != nil {
		return nil, newFetchError(fetchPhaseResponse, fmt.Errorf("read response from %q: %w", safeURL, err))
	}

	items, err := d.parser.parse(bs, resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, newFetchError(fetchPhaseResponse, fmt.Errorf("parse response from %q: %w", safeURL, err))
	}

	targets, err := targetsFromItems(d.source, items)
	if err != nil {
		return nil, newFetchError(fetchPhaseResponse, err)
	}

	return &targetGroup{
		source:  d.source,
		targets: targets,
	}, nil
}

type fetchPhase uint8

const (
	fetchPhaseRequest fetchPhase = iota + 1
	fetchPhaseQuery
	fetchPhaseResponse
)

type fetchError struct {
	phase fetchPhase
	err   error
}

func newFetchError(phase fetchPhase, err error) error {
	return &fetchError{phase: phase, err: err}
}

func (e *fetchError) Error() string {
	return e.err.Error()
}

func (e *fetchError) Unwrap() error {
	return e.err
}

func readResponseBody(r io.Reader, maxBytes int64) ([]byte, error) {
	lr := io.LimitReader(r, maxBytes+1)
	bs, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	if int64(len(bs)) > maxBytes {
		return nil, fmt.Errorf("response body exceeds limit (%d bytes)", maxBytes)
	}
	return bs, nil
}
