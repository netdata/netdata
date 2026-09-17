// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"context"
	"errors"
	"net/url"

	"github.com/stmcginnis/gofish"
)

func (c *Client) sdkConfig() gofish.ClientConfig {
	return gofish.ClientConfig{
		Endpoint:              c.origin,
		HTTPClient:            c.http,
		NoModifyTransport:     true,
		ReuseConnections:      true,
		MaxConcurrentRequests: int64(c.config.MaxConcurrentRequests),
	}
}

func (c *Client) connectSDK(
	ctx context.Context,
	cfg gofish.ClientConfig,
	stats *wireStats,
) (*gofish.APIClient, *responseData, error) {
	trace := &requestTrace{
		stats: stats,
	}
	if cfg.BasicAuth {
		// gofish reads ServiceRoot before installing its Basic credentials.
		trace.bootstrapUsername, trace.bootstrapPassword = cfg.Username, cfg.Password
	}
	client, err := gofish.ConnectContext(context.WithValue(ctx, requestTraceKey{}, trace), cfg)
	err = sdkError(err, trace)
	if trace.last != nil && !trace.last.finished {
		trace.last.finish(err)
	} else if err != nil {
		recordWireFailure(stats, classifyError(err))
	}
	return client, trace.first, err
}

func (c *Client) get(ctx context.Context, target *url.URL, stats *wireStats) (*responseData, error) {
	return c.getWithClient(ctx, c.sdk, target, stats)
}

func (c *Client) getWithClient(
	ctx context.Context,
	client *gofish.APIClient,
	target *url.URL,
	stats *wireStats,
) (*responseData, error) {
	if client == nil {
		return nil, errors.New("Redfish SDK client is not connected")
	}
	trace := &requestTrace{
		stats: stats,
	}
	response, err := client.WithContext(context.WithValue(ctx, requestTraceKey{}, trace)).Get(target.RequestURI())
	if response != nil {
		response.Body.Close()
	}
	if err = sdkError(err, trace); err != nil {
		recordWireFailure(stats, classifyError(err))
		return nil, err
	}
	return trace.last, nil
}

// SDK errors can include response bodies and credential-bearing URLs. Keep the
// status and failure class without logging untrusted remote diagnostics.
func sdkError(err error, trace *requestTrace) error {
	// http.Client may add timeout information after the transport returns.
	var network *url.Error
	if errors.As(err, &network) && network.Timeout() {
		return sanitizeTransportError(err)
	}
	if trace.failure != nil {
		return trace.failure
	}
	if err == nil {
		return nil
	}
	if isCallerContextError(err) {
		return err
	}
	if last := trace.last; last != nil && last.status >= 300 {
		return statusError{
			status: last.status,
			class:  classifyHTTPStatus(last.status),
			path:   last.url.EscapedPath(),
		}
	}
	if errors.As(err, &network) {
		return sanitizeTransportError(err)
	}
	return errors.New("Redfish SDK connection or response error")
}
