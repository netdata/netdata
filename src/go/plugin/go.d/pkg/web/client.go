// SPDX-License-Identifier: GPL-3.0-or-later

package web

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/tlscfg"
)

// ErrRedirectAttempted indicates that a redirect occurred.
var ErrRedirectAttempted = errors.New("redirect")

// Client is the configuration of the HTTP client.
// This structure is not intended to be used directly as part of a module's configuration.
// Supported configuration file formats: YAML.
type Client struct {
	// Timeout specifies a time limit for requests made by this Client.
	// Default (zero value) is no timeout. Must be set before http.Client creation.
	Timeout Duration `yaml:"timeout,omitempty" json:"timeout"`

	// NotFollowRedirect specifies the policy for handling redirects.
	// Default (zero value) is std http package default policy (stop after 10 consecutive requests).
	NotFollowRedirect bool `yaml:"not_follow_redirects,omitempty" json:"not_follow_redirects"`

	// ProxyURL specifies the URL of the proxy to use. An empty string means use the environment variables
	// HTTP_PROXY, HTTPS_PROXY and NO_PROXY (or the lowercase versions thereof) to get the URL.
	ProxyURL string `yaml:"proxy_url,omitempty" json:"proxy_url"`

	// TLSConfig specifies the TLS configuration.
	tlscfg.TLSConfig `yaml:",inline" json:""`
}

// NewHTTPClient returns a new *http.Client given a Client configuration and an error if any.
func NewHTTPClient(cfg Client) (*http.Client, error) {
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
		Proxy:               proxyFunc(cfg.ProxyURL),
		TLSClientConfig:     tlsConfig,
		DialContext:         d.DialContext,
		TLSHandshakeTimeout: cfg.Timeout.Duration(),
	}

	return &http.Client{
		Timeout:       cfg.Timeout.Duration(),
		Transport:     transport,
		CheckRedirect: redirectFunc(cfg.NotFollowRedirect),
	}, nil
}

func redirectFunc(notFollowRedirect bool) func(req *http.Request, via []*http.Request) error {
	if follow := !notFollowRedirect; follow {
		return nil
	}
	return func(_ *http.Request, _ []*http.Request) error { return ErrRedirectAttempted }
}

func proxyFunc(rawProxyURL string) func(r *http.Request) (*url.URL, error) {
	if rawProxyURL == "" {
		return http.ProxyFromEnvironment
	}
	proxyURL, _ := url.Parse(rawProxyURL)
	return http.ProxyURL(proxyURL)
}
