// SPDX-License-Identifier: GPL-3.0-or-later

package web

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"golang.org/x/net/http2"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/tlscfg"
)

// ErrRedirectAttempted indicates that a redirect occurred.
var ErrRedirectAttempted = errors.New("redirect")

// ClientConfig is the configuration of the HTTPConfig client.
// This structure is not intended to be used directly as part of a module's configuration.
// Supported configuration file formats: YAML.
type ClientConfig struct {
	// Timeout specifies a time limit for requests made by this ClientConfig.
	// Default (zero value) is no timeout. Must be set before http.Client creation.
	Timeout confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`

	// NotFollowRedirect specifies the policy for handling redirects.
	// Default (zero value) is std http package default policy (stop after 10 consecutive requests).
	NotFollowRedirect bool `yaml:"not_follow_redirects,omitempty" json:"not_follow_redirects"`

	// ProxyURL specifies the URL of the proxy to use. An empty string means use the environment variables
	// HTTP_PROXY, HTTPS_PROXY and NO_PROXY (or the lowercase versions thereof) to get the URL.
	ProxyURL string `yaml:"proxy_url,omitempty" json:"proxy_url"`

	// TLSConfig specifies the TLS configuration.
	tlscfg.TLSConfig `yaml:",inline" json:""`

	ForceHTTP2 bool `yaml:"force_http2,omitempty" json:"force_http2"`
}

// NewHTTPClient returns a new *http.Client given a ClientConfig configuration and an error if any.
func NewHTTPClient(cfg ClientConfig) (*http.Client, error) {
	var transport http.RoundTripper
	var err error

	if cfg.ForceHTTP2 {
		transport, err = newHTTP2Transport(cfg)
	} else {
		transport, err = newHTTPTransport(cfg)
	}
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Timeout:       cfg.Timeout.Duration(),
		Transport:     transport,
		CheckRedirect: redirectFunc(cfg.NotFollowRedirect),
	}

	return client, nil
}

func newHTTPTransport(cfg ClientConfig) (*http.Transport, error) {
	tlsConfig, err := tlscfg.NewTLSConfig(cfg.TLSConfig)
	if err != nil {
		return nil, fmt.Errorf("error on creating TLS config: %v", err)
	}

	if cfg.ProxyURL != "" {
		if _, err := url.Parse(cfg.ProxyURL); err != nil {
			return nil, fmt.Errorf("error on parsing proxy URL '%s': %v", cfg.ProxyURL, err)
		}
	}

	d := &net.Dialer{Timeout: cfg.Timeout.Duration()}

	transport := &http.Transport{
		TLSClientConfig:     tlsConfig,
		DialContext:         d.DialContext,
		TLSHandshakeTimeout: cfg.Timeout.Duration(),
		Proxy:               proxyFunc(cfg.ProxyURL),
	}

	return transport, nil
}

func newHTTP2Transport(cfg ClientConfig) (*http2Transport, error) {
	tlsConfig, err := tlscfg.NewTLSConfig(cfg.TLSConfig)
	if err != nil {
		return nil, fmt.Errorf("error on creating TLS config: %v", err)
	}

	d := &net.Dialer{Timeout: cfg.Timeout.Duration()}

	transport := &http2Transport{
		t2: &http2.Transport{
			TLSClientConfig: tlsConfig,
		},
		t2c: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
				return d.DialContext(ctx, network, addr)
			},
			TLSClientConfig: tlsConfig,
		},
	}

	return transport, nil
}

type http2Transport struct {
	t2  *http2.Transport
	t2c *http2.Transport
}

func (t *http2Transport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	if req.URL.Scheme == "https" {
		return t.t2.RoundTrip(req)
	}
	return t.t2c.RoundTrip(req)
}

func (t *http2Transport) CloseIdleConnections() {
	t.t2.CloseIdleConnections()
	t.t2c.CloseIdleConnections()
}

func proxyFunc(rawProxyURL string) func(r *http.Request) (*url.URL, error) {
	if rawProxyURL == "" {
		return http.ProxyFromEnvironment
	}
	proxyURL, _ := url.Parse(rawProxyURL)
	return http.ProxyURL(proxyURL)
}

func redirectFunc(notFollow bool) func(req *http.Request, via []*http.Request) error {
	if notFollow {
		return func(_ *http.Request, _ []*http.Request) error { return ErrRedirectAttempted }
	}
	return nil
}
