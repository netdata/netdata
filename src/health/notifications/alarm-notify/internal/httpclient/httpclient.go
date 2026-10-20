// SPDX-License-Identifier: GPL-3.0-or-later

package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// ResponseLimit bounds acknowledgments read from notification providers.
const ResponseLimit = 256 * 1024

// PostJSON sends a JSON body. The caller owns response validation and closing its body.
func PostJSON(
	ctx context.Context,
	client *http.Client,
	provider, endpoint string,
	headers http.Header,
	message any,
) (*http.Response, error) {
	return RequestJSON(ctx, client, provider, http.MethodPost, endpoint, headers, message)
}

// RequestJSON encodes a body and sends it with the given method.
func RequestJSON(
	ctx context.Context,
	client *http.Client,
	provider, method, endpoint string,
	headers http.Header,
	message any,
) (*http.Response, error) {
	payload, err := json.Marshal(message)
	if err != nil {
		return nil, errors.New("could not encode notification")
	}
	return Request(
		ctx,
		client,
		provider,
		method,
		endpoint,
		"application/json",
		headers,
		bytes.NewReader(payload),
	)
}

// Post sends a body without imposing provider-specific status or response rules.
func Post(
	ctx context.Context,
	client *http.Client,
	provider, endpoint, contentType string,
	headers http.Header,
	payload io.Reader,
) (*http.Response, error) {
	return Request(ctx, client, provider, http.MethodPost, endpoint, contentType, headers, payload)
}

// Request sends a body with notification headers and sanitizes transport errors.
// The caller owns response validation and closing its body.
func Request(
	ctx context.Context,
	client *http.Client,
	provider, method, endpoint, contentType string,
	headers http.Header,
	payload io.Reader,
) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, payload)
	if err != nil {
		return nil, fmt.Errorf("could not construct %s request", provider)
	}
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("User-Agent", "netdata-alarm-notify")
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, SafeError(provider, err)
	}
	return response, nil
}

// New preserves default proxy/TLS settings and disables redirects.
// The caller owns the cloned transport and must close idle connections when finished.
func New(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		Transport:     http.DefaultTransport.(*http.Transport).Clone(),
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// SafeError classifies failures without exposing credential-bearing URLs or bodies.
// The provider label must be a constant, never input or a resolved value.
func SafeError(provider string, err error) error {
	// HTTP errors include the full URL; expose only a safe failure category.
	var networkError net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return errors.New("notification canceled")
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &networkError) && networkError.Timeout():
		return errors.New("notification timed out")
	default:
		return fmt.Errorf("%s transport failed; check connectivity, TLS, and proxy settings", provider)
	}
}

// DecodeResponse decodes one size-limited JSON response without echoing its contents.
func DecodeResponse(provider string, body io.Reader, result any) error {
	data, err := ReadResponse(provider, body)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, result); err != nil {
		return fmt.Errorf("invalid %s response", provider)
	}
	return nil
}

// ReadResponse reads at most ResponseLimit bytes, rejecting larger responses.
func ReadResponse(provider string, body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, ResponseLimit+1))
	if err != nil {
		return nil, SafeError(provider, err)
	}
	if len(data) > ResponseLimit {
		return nil, fmt.Errorf("%s response exceeds the 256 KiB limit", provider)
	}
	return data, nil
}

// ValidURL checks HTTP(S) URL syntax without resolving or contacting the destination.
func ValidURL(value string, allowFragment bool) bool {
	u, err := url.Parse(value)
	return err == nil && u.Hostname() != "" && u.Opaque == "" && u.User == nil &&
		(allowFragment || u.Fragment == "") && (u.Scheme == "http" || u.Scheme == "https")
}
