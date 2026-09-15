// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const notificationResponseLimit = 256 * 1024

func sendWebhook(ctx context.Context, dst Destination, event Event, timeout time.Duration) error {
	return postJSON(ctx, dst, event, timeout)
}

func postJSON(ctx context.Context, dst Destination, message any, timeout time.Duration) error {
	endpoint, err := resolveSecret(ctx, dst.URL)
	if err != nil {
		return fmt.Errorf("destination.url: %w", err)
	}
	if err := validateURL(endpoint); err != nil {
		return err
	}
	if dst.Type == "discord" {
		endpoint, err = discordEndpoint(endpoint)
		if err != nil {
			return err
		}
	}
	var token string
	if dst.BearerToken != "" {
		token, err = resolveSecret(ctx, dst.BearerToken)
		if err != nil {
			return fmt.Errorf("destination.bearer_token: %w", err)
		}
		if strings.ContainsAny(token, "\r\n") {
			return errors.New("destination.bearer_token must not contain line breaks")
		}
	}
	client := notificationHTTPClient(timeout)
	defer client.CloseIdleConnections()
	headers := http.Header{}
	if token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	response, err := postNotificationJSON(ctx, client, dst.Type, endpoint, headers, message)
	if err != nil {
		return err
	}
	if dst.Type == "rocketchat" {
		return readRocketChatResponse(response)
	}
	// These providers acknowledge delivery through HTTP status, not the response body.
	// Close without buffering or draining an arbitrary remote body.
	defer response.Body.Close()
	accepted := response.StatusCode == http.StatusOK ||
		dst.Type == "webhook" && response.StatusCode >= 200 && response.StatusCode < 300 ||
		dst.Type == "signl4" &&
			(response.StatusCode == http.StatusCreated || response.StatusCode == http.StatusAccepted)
	if !accepted {
		return fmt.Errorf("%s returned HTTP %d", dst.Type, response.StatusCode)
	}
	return nil
}

func postNotificationJSON(
	ctx context.Context,
	client *http.Client,
	provider, endpoint string,
	headers http.Header,
	message any,
) (*http.Response, error) {
	payload, err := json.Marshal(message)
	if err != nil {
		return nil, errors.New("could not encode notification")
	}
	return postNotification(ctx, client, provider, endpoint, "application/json", headers, bytes.NewReader(payload))
}

func postNotification(
	ctx context.Context,
	client *http.Client,
	provider, endpoint, contentType string,
	headers http.Header,
	payload io.Reader,
) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, payload)
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
		return nil, notificationHTTPError(provider, err)
	}
	return response, nil
}

func notificationHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		Transport:     http.DefaultTransport.(*http.Transport).Clone(),
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func notificationHTTPError(provider string, err error) error {
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

func decodeNotificationResponse(provider string, body io.Reader, result any) error {
	data, err := io.ReadAll(io.LimitReader(body, notificationResponseLimit+1))
	if err != nil {
		return notificationHTTPError(provider, err)
	}
	if len(data) > notificationResponseLimit {
		return fmt.Errorf("%s response exceeds the 256 KiB limit", provider)
	}
	if err := json.Unmarshal(data, result); err != nil {
		return fmt.Errorf("invalid %s response", provider)
	}
	return nil
}

func resolveSecret(ctx context.Context, value string) (string, error) {
	reference, err := secretReference(value)
	if err != nil {
		return "", err
	}
	if !reference {
		return value, nil
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	scheme, operand, _ := strings.Cut(value, ":")
	operand = strings.TrimSuffix(operand, "}")
	var text string
	if scheme == "${env" {
		var ok bool
		text, ok = os.LookupEnv(operand)
		if !ok {
			return "", errors.New("secret environment variable is not set")
		}
	} else {
		data, err := os.ReadFile(operand)
		if err != nil {
			return "", errors.New("could not read secret file; check its path and access permissions")
		}
		text = string(data)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", errors.New("secret resolved to an empty value")
	}
	return text, nil
}
