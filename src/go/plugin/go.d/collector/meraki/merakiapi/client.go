// SPDX-License-Identifier: GPL-3.0-or-later

package merakiapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

// DefaultBaseURL is the global Dashboard API v1 endpoint.
const DefaultBaseURL = "https://api.meraki.com/api/v1"

// Config is client construction data, not a collector configuration schema.
// Exactly one of APIKey and BearerTokenFile must be set.
type Config struct {
	BaseURL         string
	APIKey          string
	BearerTokenFile string
}

// Client has immutable request settings and can be used concurrently. The caller
// owns the supplied HTTP transport and its idle-connection cleanup. New copies
// the HTTP client and installs this package's authenticated redirect policy.
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	request    web.RequestConfig
	regionRoot string
}

// New prepares a client without network I/O. Use web.NewHTTPClient to construct
// the supplied client with Netdata's timeout, TLS, proxy and response-limit policy.
func New(cfg Config, httpClient *http.Client) (*Client, error) {
	if httpClient == nil {
		return nil, errors.New("Meraki HTTP client is required")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	baseURL, err := url.Parse(cfg.BaseURL)
	if err != nil || !validBaseURL(baseURL) {
		return nil, errors.New("Meraki base URL must be an HTTPS /api/v1 URL without credentials, query or fragment")
	}
	baseURL.Path = "/api/v1"
	baseURL.RawPath = ""
	key := strings.TrimSpace(cfg.APIKey)
	if (key == "") == (cfg.BearerTokenFile == "") || strings.ContainsAny(key, "\r\n") {
		return nil, errors.New("Meraki requires exactly one API key or bearer token file")
	}
	request := web.RequestConfig{
		Method:          http.MethodGet,
		BearerTokenFile: cfg.BearerTokenFile,
		Headers:         map[string]string{"Accept": "application/json"},
	}
	if key != "" {
		request.Headers["Authorization"] = "Bearer " + key
	}
	cloned := *httpClient
	// Bound bodies before net/http performs redirect cleanup, including when
	// the supplied transport advertises a limit without enforcing it.
	cloned.Transport = web.NewHTTPClientWithTransport(web.ClientConfig{
		ResponseBodyLimit: confopt.ByteSize(web.HTTPClientResponseBodyLimit(httpClient)),
	}, httpClient.Transport).Transport
	c := &Client{
		baseURL:    baseURL,
		httpClient: &cloned,
		request:    request,
		regionRoot: merakiRegion(baseURL),
	}
	cloned.CheckRedirect = c.checkRedirect
	return c, nil
}

// ListOrganizations returns every organization visible to the credential.
func (c *Client) ListOrganizations(ctx context.Context) ([]Organization, error) {
	return list[Organization](ctx, c, operation{
		name:        "getOrganizations",
		pageSize:    9000,
		maxPageSize: 9000,
	}, "")
}

// ListNetworks returns every visible network in the selected organization.
func (c *Client) ListNetworks(ctx context.Context, organizationID string) ([]Network, error) {
	return list[Network](ctx, c, operation{
		name:        "getOrganizationNetworks",
		suffix:      "/networks",
		pageSize:    1000,
		maxPageSize: 100000,
	}, organizationID)
}

// ListDevices returns assigned devices, excluding unused organization inventory.
func (c *Client) ListDevices(ctx context.Context, organizationID string) ([]Device, error) {
	return list[Device](ctx, c, operation{
		name:        "getOrganizationDevices",
		suffix:      "/devices",
		pageSize:    1000,
		maxPageSize: 5000,
	}, organizationID)
}

// ListDeviceAvailabilities returns provider statuses refreshed every five minutes.
func (c *Client) ListDeviceAvailabilities(ctx context.Context, organizationID string) ([]DeviceAvailability, error) {
	return list[DeviceAvailability](ctx, c, operation{
		name:        "getOrganizationDevicesAvailabilities",
		suffix:      "/devices/availabilities",
		pageSize:    1000,
		maxPageSize: 1000,
	}, organizationID)
}

// ProbeOrganization checks operational access with one small assigned-device
// page. It deliberately ignores pagination and returns no inventory.
func (c *Client) ProbeOrganization(ctx context.Context, organizationID string) error {
	op := operation{
		name:        "getOrganizationDevices",
		suffix:      "/devices",
		pageSize:    3,
		maxPageSize: 5000,
	}
	target, err := c.operationURL(op, organizationID)
	if err != nil {
		return err
	}
	_, _, _, err = readPage[Device](ctx, c, target, op.name, web.HTTPClientResponseBodyLimit(c.httpClient))
	if err != nil {
		return err
	}
	return ctx.Err()
}

type operation struct {
	name        string
	suffix      string
	pageSize    int
	maxPageSize int
}

func (c *Client) operationURL(op operation, organizationID string) (*url.URL, error) {
	path := "/organizations"
	if op.suffix != "" {
		if organizationID == "" || organizationID == "." || organizationID == ".." {
			return nil, fmt.Errorf("%s: organization ID is required", op.name)
		}
		path += "/" + url.PathEscape(organizationID) + op.suffix
	}
	next, err := url.Parse(c.baseURL.String() + path)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid operation path", op.name)
	}
	next.RawQuery = fmt.Sprintf("perPage=%d", op.pageSize)
	return next, nil
}

func list[T any](ctx context.Context, c *Client, op operation, organizationID string) ([]T, error) {
	next, err := c.operationURL(op, organizationID)
	if err != nil {
		return nil, err
	}
	operationPath := next.EscapedPath()
	seen := make(map[string]struct{})
	result := make([]T, 0)
	limit := web.HTTPClientResponseBodyLimit(c.httpClient)
	for {
		if err := ctx.Err(); err != nil {
			return nil, safeError(op.name, "request canceled", err)
		}
		key, err := c.pageKey(next, operationPath, op.maxPageSize)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid pagination target", op.name)
		}
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("%s: cyclic pagination", op.name)
		}
		seen[key] = struct{}{}
		page, responseURL, links, err := readPage[T](ctx, c, next, op.name, limit)
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, safeError(op.name, "request canceled", err)
		}
		result = append(result, page...)
		target, found, err := nextLink(links)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid pagination Link header", op.name)
		}
		if !found {
			return result, nil
		}
		reference, err := url.Parse(target)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid pagination target", op.name)
		}
		next = responseURL.ResolveReference(reference)
	}
}

func readPage[T any](
	ctx context.Context,
	c *Client,
	target *url.URL,
	op string,
	limit int64,
) ([]T, *url.URL, []string, error) {
	req, err := c.newRequest(ctx, target)
	if err != nil {
		return nil, nil, nil, safeError(op, "cannot prepare authenticated request", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, nil, safeError(op, "HTTP request failed", err)
	}
	resp.Body = web.LimitResponseBody(resp.Body, limit)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, nil, newHTTPError(op, resp)
	}
	if encoding := strings.TrimSpace(resp.Header.Get("Content-Encoding")); encoding != "" &&
		!strings.EqualFold(encoding, "identity") {
		return nil, nil, nil, fmt.Errorf("%s: unsupported response encoding", op)
	}
	decoder := json.NewDecoder(resp.Body)
	var items []*T
	if err := decoder.Decode(&items); err != nil {
		return nil, nil, nil, safeError(op, "invalid JSON array response", err)
	}
	if items == nil {
		return nil, nil, nil, fmt.Errorf("%s: expected a non-null JSON array", op)
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, nil, nil, safeError(op, "unexpected trailing response data", err)
	}
	page := make([]T, 0, len(items))
	for _, item := range items {
		if item == nil {
			return nil, nil, nil, fmt.Errorf("%s: null array item", op)
		}
		page = append(page, *item)
	}
	responseURL := target
	if resp.Request != nil && resp.Request.URL != nil {
		responseURL = resp.Request.URL
	}
	return page, responseURL, resp.Header.Values("Link"), nil
}

func (c *Client) newRequest(ctx context.Context, target *url.URL) (*http.Request, error) {
	cfg := c.request.Copy()
	cfg.URL = target.String()
	req, err := web.NewHTTPRequest(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Header.Get("Authorization")) == "" {
		return nil, errors.New("missing bearer authentication")
	}
	return req, nil
}
