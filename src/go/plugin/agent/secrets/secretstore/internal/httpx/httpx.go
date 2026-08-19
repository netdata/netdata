// SPDX-License-Identifier: GPL-3.0-or-later

package httpx

import (
	"crypto/tls"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

var ErrResponseTooLarge = errors.New("HTTP response exceeds size limit")

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
		Timeout:       timeout,
		Transport:     transport,
		CheckRedirect: noRedirect,
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

func ReadResponseBody(r io.Reader, limit int64) ([]byte, error) {
	if limit < 0 {
		return nil, errors.New("HTTP response size limit cannot be negative")
	}
	if limit == math.MaxInt64 {
		return nil, errors.New("HTTP response size limit is too large")
	}
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return body[:limit], ErrResponseTooLarge
	}
	return body, nil
}
