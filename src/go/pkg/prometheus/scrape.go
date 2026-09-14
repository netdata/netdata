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
		// Scrape is ScrapeContext with a background context (for callers that do
		// not thread one).
		Scrape() (MetricFamilies, error)
		// ScrapeContext scrapes and assembles typed MetricFamilies, honoring ctx for
		// the fetch (cancellation/deadline). This is the no-buffer fast path.
		ScrapeContext(ctx context.Context) (MetricFamilies, error)
		// ScrapeSamples scrapes and returns the flat, classified sample stream plus
		// per-family HELP, with the selector applied, before typed-family assembly.
		// The caller owns the samples and may relabel them in place, then fold the
		// result with [Assemble]. This is the seam a metric-relabeling step plugs into.
		//
		// Samples are in exposition order, with one exception: a _sum/_count sample
		// whose family type is not yet known (its # TYPE, first bucket, or first
		// quantile has not appeared) is deferred and delivered once the type resolves,
		// or at end of scrape — so it may arrive after a later, unrelated sample.
		ScrapeSamples(ctx context.Context) (SampleBatch, error)
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
	return p.ScrapeContext(context.Background())
}

func (p *prometheus) ScrapeContext(ctx context.Context) (MetricFamilies, error) {
	p.buf.Reset()

	if err := p.src.fetch(ctx, p.buf); err != nil {
		return nil, err
	}

	return p.parser.parseToMetricFamilies(p.buf.Bytes())
}

func (p *prometheus) ScrapeSamples(ctx context.Context) (SampleBatch, error) {
	p.buf.Reset()

	if err := p.src.fetch(ctx, p.buf); err != nil {
		return SampleBatch{}, err
	}

	return p.parser.parseToSamples(p.buf.Bytes())
}
