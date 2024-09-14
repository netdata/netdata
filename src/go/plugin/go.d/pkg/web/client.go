// SPDX-License-Identifier: GPL-3.0-or-later

package web

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
)

type Client struct {
	httpClient *http.Client
	onNokCode  func(resp *http.Response) (bool, error)
}

func DoHTTP(cl *http.Client) *Client {
	return &Client{
		httpClient: cl,
	}
}

func (c *Client) OnNokCode(fn func(resp *http.Response) (bool, error)) *Client {
	c.onNokCode = fn
	return c
}

func (c *Client) RequestJSON(req *http.Request, in any) error {
	return c.Request(req, func(body io.Reader) error {
		return json.NewDecoder(body).Decode(in)
	})
}

func (c *Client) RequestXML(req *http.Request, in any, opts ...func(dec *xml.Decoder)) error {
	return c.Request(req, func(body io.Reader) error {
		dec := xml.NewDecoder(body)
		for _, opt := range opts {
			opt(dec)
		}
		return dec.Decode(in)
	})
}

func (c *Client) Request(req *http.Request, parse func(body io.Reader) error) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error on HTTP request to '%s': %w", req.URL, err)
	}

	defer CloseBody(resp)

	if resp.StatusCode != http.StatusOK {
		if err := c.handleNokCode(req, resp); err != nil {
			return err
		}
	}

	if parse != nil {
		if err := parse(resp.Body); err != nil {
			return fmt.Errorf("error on parsing response from '%s': %w", req.URL, err)
		}
	}

	return nil
}

func (c *Client) handleNokCode(req *http.Request, resp *http.Response) error {
	if c.onNokCode != nil {
		handled, err := c.onNokCode(resp)
		if err != nil {
			return fmt.Errorf("'%s' returned HTTP status code: %d (%w)", req.URL, resp.StatusCode, err)
		}
		if handled {
			return nil
		}
	}
	return fmt.Errorf("'%s' returned HTTP status code: %d", req.URL, resp.StatusCode)
}

func CloseBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}
