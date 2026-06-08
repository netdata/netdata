// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

type (
	// Prometheus is a helper for scrape and parse prometheus format metrics.
	Prometheus interface {
		// ScrapeSeries and parse prometheus format metrics
		ScrapeSeries() (Series, error)
		Scrape() (MetricFamilies, error)
		// ScrapeWithTransform scrapes, runs transform on every sample (a flat
		// [Sample] stream before typed-family assembly, with the selector already
		// applied), then assembles the kept, transformed samples into MetricFamilies.
		// A nil transform behaves exactly like Scrape. The result aliases reused
		// buffers, valid until the next scrape on this instance (same as Scrape).
		//
		// Samples reach transform in exposition order, with one exception: a
		// _sum/_count sample whose family type is not yet known (its # TYPE, first
		// bucket, or first quantile has not appeared) is deferred and delivered once
		// the type resolves, or at end of scrape — so it may arrive after a later,
		// unrelated sample. A per-sample (stateless) transform is unaffected.
		ScrapeWithTransform(ctx context.Context, transform SampleTransform) (MetricFamilies, error)
		HTTPClient() *http.Client
	}

	prometheus struct {
		client *http.Client
		src    fetcher

		parser promTextParser

		buf *bytes.Buffer
	}
)

// New creates a Prometheus instance.
func New(client *http.Client, request web.RequestConfig) Prometheus {
	return NewWithSelector(client, request, nil)
}

// NewWithSelector creates a Prometheus instance with the selector.
func NewWithSelector(client *http.Client, request web.RequestConfig, sr selector.Selector) Prometheus {
	p := &prometheus{
		client: client,
		buf:    bytes.NewBuffer(make([]byte, 0, 16000)),
		parser: promTextParser{sr: sr},
	}

	if v, err := url.Parse(request.URL); err == nil && v.Scheme == "file" {
		p.src = &fileFetcher{path: filepath.Join(v.Host, v.Path)}
	} else {
		p.src = &httpFetcher{client: client, request: request}
	}

	return p
}

func (p *prometheus) HTTPClient() *http.Client {
	return p.client
}

// ScrapeSeries scrapes metrics, parses and sorts
func (p *prometheus) ScrapeSeries() (Series, error) {
	p.buf.Reset()

	if err := p.src.fetch(context.Background(), p.buf); err != nil {
		return nil, err
	}

	return p.parser.parseToSeries(p.buf.Bytes())
}

func (p *prometheus) Scrape() (MetricFamilies, error) {
	return p.ScrapeWithTransform(context.Background(), nil)
}

func (p *prometheus) ScrapeWithTransform(ctx context.Context, transform SampleTransform) (MetricFamilies, error) {
	p.buf.Reset()

	if err := p.src.fetch(ctx, p.buf); err != nil {
		return nil, err
	}

	return p.parser.parseToMetricFamilies(p.buf.Bytes(), transform)
}
