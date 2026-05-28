// SPDX-License-Identifier: GPL-3.0-or-later

package httpx

import (
	"crypto/tls"
	"net/http"
	"strings"
	"time"
)

func noRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

func APIClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

func NoProxyClient(timeout time.Duration) *http.Client {
	transport := cloneDefaultTransport()
	transport.Proxy = nil

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

func VaultClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: noRedirect,
	}
}

func VaultInsecureClient(timeout time.Duration) *http.Client {
	transport := cloneDefaultTransport()
	if transport.TLSClientConfig != nil {
		transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	} else {
		transport.TLSClientConfig = &tls.Config{}
	}
	transport.TLSClientConfig.InsecureSkipVerify = true

	return &http.Client{
		Timeout:       timeout,
		Transport:     transport,
		CheckRedirect: noRedirect,
	}
}

func cloneDefaultTransport() *http.Transport {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok || transport == nil {
		return &http.Transport{}
	}
	return transport.Clone()
}

func TruncateBody(body []byte) string {
	const maxLen = 200
	s := strings.TrimSpace(string(body))
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
