// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const (
	urlPathApiAuth       = "/api/auth"
	urlPathApiAuthCheck  = "/api/auth/check"
	urlPathApiAuthLogout = "/api/auth/logout"
)

func (c *Ceph) authLogin() (string, error) {
	// https://docs.ceph.com/en/reef/mgr/ceph_api/#post--api-auth

	req, err := func() (*http.Request, error) {
		var credentials = struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}{
			Username: c.Username,
			Password: c.Password,
		}

		bs, err := json.Marshal(credentials)
		if err != nil {
			return nil, err
		}

		req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathApiAuth)
		if err != nil {
			return nil, err
		}

		body := bytes.NewReader(bs)

		req.Body = io.NopCloser(body)
		req.ContentLength = int64(body.Len())
		req.Method = http.MethodPost
		req.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
		req.Header.Set("Content-Type", "application/json")

		return req, nil
	}()

	if err != nil {
		return "", err
	}

	var token struct {
		Token string `json:"token"`
	}

	if err := c.webClient(201).RequestJSON(req, &token); err != nil {
		return "", err
	}

	if token.Token == "" {
		return "", errors.New("empty token")
	}

	return token.Token, nil
}

func (c *Ceph) authCheck() (bool, error) {
	// https://docs.ceph.com/en/reef/mgr/ceph_api/#post--api-auth-check
	if c.token == "" {
		return false, nil
	}

	req, err := func() (*http.Request, error) {
		var token = struct {
			Token string `json:"token"`
		}{
			Token: c.token,
		}

		bs, err := json.Marshal(token)
		if err != nil {
			return nil, err
		}

		req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathApiAuthCheck)
		if err != nil {
			return nil, err
		}

		body := bytes.NewReader(bs)

		req.Body = io.NopCloser(body)
		req.ContentLength = int64(body.Len())
		req.URL.RawQuery = url.Values{"token": {c.token}}.Encode() // TODO: it seems not necessary?
		req.Method = http.MethodPost
		req.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}()
	if err != nil {
		return false, err
	}

	var resp struct {
		Username    string         `json:"username"`
		Permissions map[string]any `json:"permissions"`
	}

	if err := c.webClient().RequestJSON(req, &resp); err != nil {
		return false, err
	}

	return resp.Username != "" && resp.Permissions != nil, nil
}

func (c *Ceph) authLogout() error {
	// https://docs.ceph.com/en/reef/mgr/ceph_api/#post--api-auth-logout

	if c.token == "" {
		return nil
	}
	defer func() { c.token = "" }()

	req, err := func() (*http.Request, error) {
		req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathApiAuthLogout)
		if err != nil {
			return nil, err
		}

		req.Method = http.MethodPost
		req.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
		req.Header.Set("Authorization", "Bearer "+c.token)
		return req, nil
	}()
	if err != nil {
		return err
	}

	return c.webClient().Request(req, nil)
}
