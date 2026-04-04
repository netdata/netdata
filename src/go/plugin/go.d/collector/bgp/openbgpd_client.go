// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

type openbgpdPathLayout struct {
	name      string
	neighbors string
	rib       string
}

var openbgpdPathLayouts = []openbgpdPathLayout{
	{
		name:      "bgplgd",
		neighbors: "/neighbors",
		rib:       "/rib",
	},
	{
		name:      "state_server",
		neighbors: "/v1/bgpd/show/neighbor",
		rib:       "/v1/bgpd/show/rib/detail",
	},
}

type openbgpdClient struct {
	baseURL string
	client  *http.Client

	mu     sync.Mutex
	layout *openbgpdPathLayout
}

type openbgpdPathError struct {
	err error
}

func (e openbgpdPathError) Error() string { return e.err.Error() }
func (e openbgpdPathError) Unwrap() error { return e.err }

var errOpenBGPDFilteredRIBUnsupported = errors.New("OpenBGPD filtered RIB queries are not supported for the detected API layout")
var errOpenBGPDFilteredRIBFamilyUnsupported = errors.New("OpenBGPD filtered RIB queries are not supported for this family")

func newOpenBGPDClient(cfg Config) *openbgpdClient {
	return &openbgpdClient{
		baseURL: cfg.APIURL,
		client: &http.Client{
			Timeout: cfg.Timeout.Duration(),
		},
	}
}

func (c *openbgpdClient) Close() error { return nil }

func (c *openbgpdClient) Neighbors() ([]byte, error) {
	return c.query("neighbors", "")
}

func (c *openbgpdClient) RIB() ([]byte, error) {
	return c.query("rib", "")
}

func (c *openbgpdClient) RIBFiltered(family string) ([]byte, error) {
	family = strings.TrimSpace(family)
	if family == "" {
		return c.RIB()
	}
	if !isOpenBGPDFilteredRIBFamilySupported(family) {
		return nil, fmt.Errorf("%w: %s", errOpenBGPDFilteredRIBFamilyUnsupported, family)
	}

	values := url.Values{}
	values.Set("af", family)

	for attempt := 0; attempt < 2; attempt++ {
		if c.cachedLayout() == nil {
			if _, err := c.Neighbors(); err != nil {
				return nil, err
			}
		}

		layout := c.cachedLayout()
		if layout == nil || layout.name != "bgplgd" {
			return nil, errOpenBGPDFilteredRIBUnsupported
		}

		data, err := c.request(layout.path("rib"), values.Encode())
		if isOpenBGPDPathError(err) {
			c.clearLayout(layout.name)
			continue
		}
		return data, err
	}

	return nil, errOpenBGPDFilteredRIBUnsupported
}

func (c *openbgpdClient) query(kind, rawQuery string) ([]byte, error) {
	if layout := c.cachedLayout(); layout != nil {
		data, err := c.request(layout.path(kind), rawQuery)
		if !isOpenBGPDPathError(err) {
			return data, err
		}
		c.clearLayout(layout.name)
	}

	var firstErr error
	for i := range openbgpdPathLayouts {
		layout := openbgpdPathLayouts[i]
		data, err := c.request(layout.path(kind), rawQuery)
		if err == nil {
			c.storeLayout(layout)
			return data, nil
		}
		if !isOpenBGPDPathError(err) {
			return nil, err
		}
		if firstErr == nil {
			firstErr = err
		}
	}

	if firstErr != nil {
		return nil, firstErr
	}
	return nil, fmt.Errorf("OpenBGPD %s endpoint not found under %s", kind, c.baseURL)
}

func (c *openbgpdClient) request(path, rawQuery string) ([]byte, error) {
	reqURL := c.baseURL + path
	if rawQuery != "" {
		reqURL += "?" + rawQuery
	}

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, readErr
	}

	switch resp.StatusCode {
	case http.StatusOK:
		return body, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("%w: HTTP %d from %s", fs.ErrPermission, resp.StatusCode, req.URL.String())
	case http.StatusNotFound:
		return nil, openbgpdPathError{err: fmt.Errorf("OpenBGPD endpoint %s returned HTTP %d", req.URL.String(), resp.StatusCode)}
	default:
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return nil, fmt.Errorf("OpenBGPD endpoint %s returned HTTP %d: %s", req.URL.String(), resp.StatusCode, msg)
	}
}

func isOpenBGPDPathError(err error) bool {
	var pathErr openbgpdPathError
	return errors.As(err, &pathErr)
}

func (c *openbgpdClient) cachedLayout() *openbgpdPathLayout {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.layout == nil {
		return nil
	}
	layout := *c.layout
	return &layout
}

func (c *openbgpdClient) storeLayout(layout openbgpdPathLayout) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.layout = &layout
}

func (c *openbgpdClient) clearLayout(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.layout != nil && c.layout.name == name {
		c.layout = nil
	}
}

func (l openbgpdPathLayout) path(kind string) string {
	switch kind {
	case "rib":
		return l.rib
	default:
		return l.neighbors
	}
}

func isOpenBGPDFilteredRIBFamilySupported(family string) bool {
	switch family {
	case "ipv4", "ipv6", "vpnv4", "vpnv6":
		return true
	default:
		return false
	}
}
