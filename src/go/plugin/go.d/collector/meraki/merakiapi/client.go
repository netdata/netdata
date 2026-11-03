// SPDX-License-Identifier: GPL-3.0-or-later

// Package merakiapi reads organization inventory and device availability from
// the Meraki Dashboard API v1. It returns provider data as-is; interpreting it is
// the caller's job.
package merakiapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

// DefaultBaseURL is the global Dashboard API v1 endpoint.
const DefaultBaseURL = "https://api.meraki.com/api/v1"

// minPageSize is the smallest perPage value Meraki accepts.
const minPageSize = 3

// Config holds client construction data. Exactly one of APIKey and
// BearerTokenFile must be set.
type Config struct {
	BaseURL         string
	APIKey          string
	BearerTokenFile string
}

// Client is safe for concurrent use. The caller owns the supplied HTTP client:
// its timeout, TLS, proxy, response body limit and idle connections.
type Client struct {
	baseURL    *url.URL
	regionRoot string // regional root of an official API base, "" for any other origin
	httpClient *http.Client
	request    web.RequestConfig
}

// New prepares a client without network I/O. It copies httpClient to install the
// Meraki redirect policy and leaves the original unchanged.
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

	c := &Client{
		baseURL:    baseURL,
		regionRoot: merakiRegion(baseURL),
		request:    request,
	}
	hc := *httpClient
	hc.CheckRedirect = c.checkRedirect
	c.httpClient = &hc
	return c, nil
}

func validBaseURL(u *url.URL) bool {
	return u != nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil &&
		u.Opaque == "" && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" &&
		strings.TrimRight(u.EscapedPath(), "/") == "/api/v1"
}

// operation is one paginated list endpoint.
type operation struct {
	name        string // OpenAPI operationId, used in errors
	path        string // path below /organizations/{organizationId}; empty for /organizations
	pageSize    int    // perPage of the first request
	maxPageSize int    // largest perPage accepted in a next link
}

var (
	opOrganizations = operation{
		name:        "getOrganizations",
		pageSize:    9000,
		maxPageSize: 9000,
	}
	opNetworks = operation{
		name:        "getOrganizationNetworks",
		path:        "/networks",
		pageSize:    1000,
		maxPageSize: 100000,
	}
	opDevices = operation{
		name:        "getOrganizationDevices",
		path:        "/devices",
		pageSize:    1000,
		maxPageSize: 5000,
	}
	opDeviceAvailabilities = operation{
		name:        "getOrganizationDevicesAvailabilities",
		path:        "/devices/availabilities",
		pageSize:    1000,
		maxPageSize: 1000,
	}
)

// ListOrganizations returns every organization visible to the credential.
func (c *Client) ListOrganizations(ctx context.Context) ([]Organization, error) {
	return list[Organization](ctx, c, opOrganizations, "")
}

// ListNetworks returns every network of the organization.
func (c *Client) ListNetworks(ctx context.Context, organizationID string) ([]Network, error) {
	return list[Network](ctx, c, opNetworks, organizationID)
}

// ListDevices returns the devices assigned to the organization's networks.
func (c *Client) ListDevices(ctx context.Context, organizationID string) ([]Device, error) {
	return list[Device](ctx, c, opDevices, organizationID)
}

// ListDeviceAvailabilities returns the provider status of the organization's
// devices.
func (c *Client) ListDeviceAvailabilities(ctx context.Context, organizationID string) ([]DeviceAvailability, error) {
	return list[DeviceAvailability](ctx, c, opDeviceAvailabilities, organizationID)
}

// ProbeOrganization checks access to the organization by reading one minimal
// assigned-device page. It ignores pagination and returns no inventory. Listing
// organizations cannot prove access: Meraki exempts that call from the
// organization's API source-IP restrictions.
func (c *Client) ProbeOrganization(ctx context.Context, organizationID string) error {
	op := opDevices
	op.pageSize = minPageSize
	target, err := c.operationURL(op, organizationID)
	if err != nil {
		return err
	}
	_, err = fetchPage[Device](ctx, c, target, op.name)
	return err
}

func (c *Client) operationURL(op operation, organizationID string) (*url.URL, error) {
	path := "/organizations"
	if op.path != "" {
		// PathEscape keeps dot segments, which would climb out of the organization.
		if organizationID == "" || organizationID == "." || organizationID == ".." {
			return nil, fmt.Errorf("%s: invalid organization ID", op.name)
		}
		path += "/" + url.PathEscape(organizationID) + op.path
	}
	target, err := url.Parse(c.baseURL.String() + path)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid operation path", op.name)
	}
	target.RawQuery = "perPage=" + strconv.Itoa(op.pageSize)
	return target, nil
}

// list follows rel="next" links to the last page. It returns the complete list,
// or nil and an error: a failed later page never looks like a shorter list. The
// pages are not an atomic snapshot; the provider can change data between them.
func list[T any](ctx context.Context, c *Client, op operation, organizationID string) ([]T, error) {
	target, err := c.operationURL(op, organizationID)
	if err != nil {
		return nil, err
	}
	operationPath := target.EscapedPath()
	visited := make(map[string]bool)
	items := make([]T, 0)

	for {
		if err := ctx.Err(); err != nil {
			return nil, safeError(op.name, "request canceled", err)
		}
		key, err := c.pageKey(target, operationPath, op.maxPageSize)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid pagination target", op.name)
		}
		if visited[key] {
			return nil, fmt.Errorf("%s: cyclic pagination", op.name)
		}
		visited[key] = true

		p, err := fetchPage[T](ctx, c, target, op.name)
		if err != nil {
			return nil, err
		}
		items = append(items, p.items...)

		next, found, err := p.next()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op.name, err)
		}
		if !found {
			return items, nil
		}
		target = next
	}
}

// page is one decoded response.
type page[T any] struct {
	items []T
	url   *url.URL // final request URL after redirects, the base of relative links
	links []string // raw Link header values
}

// next resolves the page's rel="next" link; found is false on the last page.
func (p page[T]) next() (next *url.URL, found bool, err error) {
	target, found, err := nextLink(p.links)
	if err != nil {
		return nil, false, errors.New("invalid pagination Link header")
	}
	if !found {
		return nil, false, nil
	}
	ref, err := url.Parse(target)
	if err != nil {
		return nil, false, errors.New("invalid pagination target")
	}
	return p.url.ResolveReference(ref), true, nil
}

// fetchPage sends one authenticated GET and decodes its JSON array.
func fetchPage[T any](ctx context.Context, c *Client, target *url.URL, opName string) (page[T], error) {
	return fetchDecodedPage(ctx, c, target, opName, decodeArray[T])
}

// fetchDecodedPage shares request safety between array and items-envelope responses.
func fetchDecodedPage[T any](ctx context.Context, c *Client, target *url.URL, opName string, decode func(io.Reader, string) ([]T, error)) (page[T], error) {
	req, err := c.newRequest(ctx, target)
	if err != nil {
		return page[T]{}, safeError(opName, "cannot prepare authenticated request", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return page[T]{}, transportError(opName, err)
	}
	// Clients built by web.NewHTTPClient already bound bodies; this keeps the
	// bound for any other http.Client.
	resp.Body = web.LimitResponseBody(resp.Body, web.HTTPClientResponseBodyLimit(c.httpClient))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return page[T]{}, newHTTPError(opName, resp)
	}
	// net/http decompresses gzip itself; any encoding still present is unreadable.
	if enc := strings.TrimSpace(resp.Header.Get("Content-Encoding")); enc != "" && !strings.EqualFold(enc, "identity") {
		return page[T]{}, fmt.Errorf("%s: unsupported response encoding", opName)
	}
	items, err := decode(resp.Body, opName)
	if err != nil {
		return page[T]{}, err
	}
	if err := ctx.Err(); err != nil {
		return page[T]{}, safeError(opName, "request canceled", err)
	}
	finalURL := target
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL
	}
	return page[T]{
		items: items,
		url:   finalURL,
		links: resp.Header.Values("Link"),
	}, nil
}

// decodeArray requires exactly one non-null JSON array of non-null items.
func decodeArray[T any](r io.Reader, opName string) ([]T, error) {
	dec := json.NewDecoder(r)
	var ptrs []*T
	if err := dec.Decode(&ptrs); err != nil {
		return nil, safeError(opName, "invalid JSON array response", err)
	}
	if ptrs == nil {
		return nil, fmt.Errorf("%s: expected a non-null JSON array", opName)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, safeError(opName, "unexpected trailing response data", err)
	}
	items := make([]T, 0, len(ptrs))
	for _, ptr := range ptrs {
		if ptr == nil {
			return nil, fmt.Errorf("%s: null array item", opName)
		}
		items = append(items, *ptr)
	}
	return items, nil
}

// newRequest builds an authenticated GET. A bearer token file is read on every
// call, so a rotated token applies without a restart.
func (c *Client) newRequest(ctx context.Context, target *url.URL) (*http.Request, error) {
	cfg := c.request.Copy()
	cfg.URL = target.String()
	req, err := web.NewHTTPRequest(ctx, cfg)
	if err != nil {
		return nil, err
	}
	// web skips a missing optional Kubernetes service-account token file without
	// an error; a Meraki request must never be sent unauthenticated.
	if strings.TrimSpace(req.Header.Get("Authorization")) == "" {
		return nil, errors.New("missing bearer authentication")
	}
	return req, nil
}
