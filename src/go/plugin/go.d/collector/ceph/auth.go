// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

type (
	authLoginResp struct {
		Token string `json:"token"`
	}
	authCheckResp struct {
		Username    string         `json:"username"`
		Permissions map[string]any `json:"permissions"`
	}
)

func (c *Collector) authLogin() (string, error) {
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
		req.Header.Set("Accept", hdrAcceptVersion)
		req.Header.Set("Content-Type", hdrContentTypeJson)

		return req, nil
	}()
	if err != nil {
		return "", err
	}

	var tok authLoginResp

	if err := c.webClient(201).RequestJSON(req, &tok); err != nil {
		return "", err
	}

	if tok.Token == "" {
		return "", errors.New("empty token")
	}

	return tok.Token, nil
}

func (c *Collector) authCheck() (bool, error) {
	// https://docs.ceph.com/en/reef/mgr/ceph_api/#post--api-auth-check
	if c.token == "" {
		return false, nil
	}

	req, err := func() (*http.Request, error) {
		bs, err := json.Marshal(authLoginResp{Token: c.token})
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
		req.Header.Set("Accept", hdrAcceptVersion)
		req.Header.Set("Content-Type", hdrContentTypeJson)
		return req, nil
	}()
	if err != nil {
		return false, err
	}

	var resp authCheckResp

	if err := c.webClient().RequestJSON(req, &resp); err != nil {
		return false, err
	}

	return resp.Username != "", nil
}

func (c *Collector) authLogout() error {
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
		req.Header.Set("Accept", hdrAcceptVersion)
		req.Header.Set("Authorization", "Bearer "+c.token)
		return req, nil
	}()
	if err != nil {
		return err
	}

	return c.webClient().Request(req, nil)
}
