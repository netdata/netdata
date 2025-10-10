// Package openmetrics provides a reusable OpenMetrics/Prometheus text protocol client.
// SPDX-License-Identifier: GPL-3.0-or-later

package openmetrics

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

const defaultAcceptHeader = "text/plain;version=0.0.4;q=1,*/*;q=0.1"

// Config describes how to communicate with an OpenMetrics endpoint.
type Config struct {
	HTTPConfig web.HTTPConfig
	// Accept allows overriding the Accept header advertised to the endpoint.
	Accept string
}

// Client fetches and parses OpenMetrics data from a single endpoint.
type Client struct {
	cfg        Config
	httpClient *http.Client
	request    web.RequestConfig

	acceptHeader string
}

// NewClient constructs a client with a freshly created HTTP transport.
func NewClient(cfg Config) (*Client, error) {
	if time.Duration(cfg.HTTPConfig.ClientConfig.Timeout) <= 0 {
		cfg.HTTPConfig.ClientConfig.Timeout = confopt.Duration(10 * time.Second)
	}

	httpClient, err := web.NewHTTPClient(cfg.HTTPConfig.ClientConfig)
	if err != nil {
		return nil, fmt.Errorf("openmetrics protocol: creating http client failed: %w", err)
	}

	return NewClientWithHTTP(cfg, httpClient)
}

// NewClientWithHTTP constructs a client using the provided *http.Client instance (useful for tests).
func NewClientWithHTTP(cfg Config, httpClient *http.Client) (*Client, error) {
	if httpClient == nil {
		return nil, errors.New("openmetrics protocol: http client is required")
	}

	trimmedURL := strings.TrimSpace(cfg.HTTPConfig.RequestConfig.URL)
	if trimmedURL == "" {
		return nil, errors.New("openmetrics protocol: url is required")
	}

	accept := strings.TrimSpace(cfg.Accept)
	if accept == "" {
		accept = defaultAcceptHeader
	}

	return &Client{
		cfg:          cfg,
		httpClient:   httpClient,
		request:      cfg.HTTPConfig.RequestConfig.Copy(),
		acceptHeader: accept,
	}, nil
}

// FetchSeries retrieves the metrics and returns them as a list of series samples.
func (c *Client) FetchSeries(ctx context.Context, sr selector.Selector) (prometheus.Series, error) {
	payload, err := c.fetch(ctx)
	if err != nil {
		return nil, err
	}

	parser := seriesParser{selector: sr}
	return parser.parse(payload)
}

func (c *Client) fetch(ctx context.Context) ([]byte, error) {
	req, err := web.NewHTTPRequest(c.request)
	if err != nil {
		return nil, fmt.Errorf("openmetrics protocol: building request failed: %w", err)
	}

	req = req.WithContext(ctx)
	req.Header.Set("Accept", c.acceptHeader)
	// Prefer gzip for large payloads but fall back gracefully.
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("openmetrics protocol: request cancelled: %w", ctx.Err())
		}
		return nil, fmt.Errorf("openmetrics protocol: request failed: %w", err)
	}
	defer web.CloseBody(resp)

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("openmetrics protocol: unexpected status %d: %s", resp.StatusCode, string(snippet))
	}

	var reader io.Reader = resp.Body
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("openmetrics protocol: creating gzip reader failed: %w", err)
		}
		defer gz.Close()
		reader = gz
	}

	buf := bytes.Buffer{}
	if _, err := buf.ReadFrom(reader); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("openmetrics protocol: read cancelled: %w", ctx.Err())
		}
		return nil, fmt.Errorf("openmetrics protocol: reading response failed: %w", err)
	}

	return buf.Bytes(), nil
}
