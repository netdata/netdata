// SPDX-License-Identifier: GPL-3.0-or-later

package xquik

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

const maxProfileResponseBytes = 1 << 20

type profileClient interface {
	Profile(context.Context, string) (profile, error)
}

type httpProfileClient struct {
	request    web.RequestConfig
	apiKey     string
	httpClient *http.Client
}

func newProfileClient(cfg Config, httpClient *http.Client) profileClient {
	return &httpProfileClient{
		request:    web.RequestConfig{URL: cfg.URL, Method: http.MethodGet},
		apiKey:     cfg.APIKey,
		httpClient: httpClient,
	}
}

func (c *httpProfileClient) Profile(ctx context.Context, user string) (profile, error) {
	req, err := web.NewHTTPRequestWithPath(c.request, "x/users/"+user)
	if err != nil {
		return profile{}, fmt.Errorf("create Xquik profile request: %w", err)
	}
	req = req.WithContext(ctx)
	req.Header.Set("x-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return profile{}, fmt.Errorf("request Xquik profile: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return profile{}, fmt.Errorf("request Xquik profile: HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxProfileResponseBytes+1))
	if err != nil {
		return profile{}, fmt.Errorf("read Xquik profile response: %w", err)
	}
	if len(body) > maxProfileResponseBytes {
		return profile{}, fmt.Errorf("read Xquik profile response: body exceeds %d bytes", maxProfileResponseBytes)
	}

	var result profile
	if err := json.Unmarshal(body, &result); err != nil {
		return profile{}, fmt.Errorf("decode Xquik profile response: %w", err)
	}
	if err := result.normalizeAndValidate(); err != nil {
		return profile{}, fmt.Errorf("validate Xquik profile response: %w", err)
	}
	return result, nil
}

type profile struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Name          string `json:"name"`
	Followers     *int64 `json:"followers"`
	Following     *int64 `json:"following"`
	StatusesCount *int64 `json:"statusesCount"`
	Verified      *bool  `json:"verified"`
}

func (p *profile) normalizeAndValidate() error {
	p.ID = strings.TrimSpace(p.ID)
	p.Username = strings.TrimSpace(p.Username)
	p.Name = strings.TrimSpace(p.Name)

	if p.ID == "" || p.Username == "" || p.Name == "" {
		return fmt.Errorf("required fields id, username, and name must be non-empty")
	}
	for name, value := range map[string]*int64{
		"followers":     p.Followers,
		"following":     p.Following,
		"statusesCount": p.StatusesCount,
	} {
		if value != nil && *value < 0 {
			return fmt.Errorf("field %s must not be negative", name)
		}
	}
	return nil
}
