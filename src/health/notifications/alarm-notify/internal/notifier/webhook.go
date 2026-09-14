// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

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
	payload, err := json.Marshal(message)
	if err != nil {
		return errors.New("could not encode notification")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("could not construct %s request", dst.Type)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "netdata-alarm-notify")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{
		Timeout:       timeout,
		Transport:     http.DefaultTransport.(*http.Transport).Clone(),
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer client.CloseIdleConnections()
	response, err := client.Do(request)
	if err != nil {
		// HTTP errors include the full URL; expose only a safe failure category.
		var networkError net.Error
		switch {
		case errors.Is(err, context.Canceled):
			return errors.New("notification canceled")
		case errors.Is(err, context.DeadlineExceeded), errors.As(err, &networkError) && networkError.Timeout():
			return errors.New("notification timed out")
		default:
			return fmt.Errorf("%s transport failed; check connectivity, TLS, and proxy settings", dst.Type)
		}
	}
	// These providers acknowledge delivery through HTTP status, not the response body.
	// Close without buffering or draining an arbitrary remote body.
	defer response.Body.Close()
	accepted := response.StatusCode >= 200 && response.StatusCode < 300
	if dst.Type == "slack" {
		accepted = response.StatusCode == http.StatusOK
	}
	if !accepted {
		return fmt.Errorf("%s returned HTTP %d", dst.Type, response.StatusCode)
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
