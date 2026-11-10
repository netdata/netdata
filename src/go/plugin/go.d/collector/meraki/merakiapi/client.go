// SPDX-License-Identifier: GPL-3.0-or-later

// Package merakiapi reads organization inventory, device availability and
// traffic from the Meraki Dashboard API v1. It returns provider data as-is;
// interpreting it is the caller's job.
package merakiapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

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

// operation is one read-only Dashboard API endpoint.
type operation struct {
	name        string // OpenAPI operationId, used in errors
	path        string // OpenAPI path below /api/v1, with at most one {parameter}
	pageSize    int    // perPage of the first request; 0 for an unpaginated operation
	maxPageSize int    // largest perPage accepted in a next link
	itemsObject bool   // the response wraps its array in an {"items": [...]} object
}

var (
	opOrganizations = operation{
		name:        "getOrganizations",
		path:        "/organizations",
		pageSize:    9000,
		maxPageSize: 9000,
	}
	opNetworks = operation{
		name:        "getOrganizationNetworks",
		path:        "/organizations/{organizationId}/networks",
		pageSize:    1000,
		maxPageSize: 100000,
	}
	opDevices = operation{
		name:        "getOrganizationDevices",
		path:        "/organizations/{organizationId}/devices",
		pageSize:    1000,
		maxPageSize: 5000,
	}
	opDeviceAvailabilities = operation{
		name:        "getOrganizationDevicesAvailabilities",
		path:        "/organizations/{organizationId}/devices/availabilities",
		pageSize:    1000,
		maxPageSize: 1000,
	}
	opSwitchPortsUsageHistory = operation{
		name:        "getOrganizationSwitchPortsUsageHistoryByDeviceByInterval",
		path:        "/organizations/{organizationId}/switch/ports/usage/history/byDevice/byInterval",
		pageSize:    50,
		maxPageSize: 50,
		itemsObject: true,
	}
	opSwitchPortsPackets = operation{
		name: "getDeviceSwitchPortsStatusesPackets",
		path: "/devices/{serial}/switch/ports/statuses/packets",
	}
	opApplianceUplinkStatuses = operation{
		name:        "getOrganizationApplianceUplinkStatuses",
		path:        "/organizations/{organizationId}/appliance/uplink/statuses",
		pageSize:    1000,
		maxPageSize: 1000,
	}
	opUplinkLossAndLatency = operation{
		name: "getOrganizationDevicesUplinksLossAndLatency",
		path: "/organizations/{organizationId}/devices/uplinksLossAndLatency",
	}
	opApplianceUplinksUsage = operation{
		name: "getOrganizationApplianceUplinksUsageByNetwork",
		path: "/organizations/{organizationId}/appliance/uplinks/usage/byNetwork",
	}
)

// ListOrganizations returns every organization visible to the credential.
func (c *Client) ListOrganizations(ctx context.Context) ([]Organization, error) {
	return list[Organization](ctx, c, opOrganizations, "", nil)
}

// ListNetworks returns every network of the organization.
func (c *Client) ListNetworks(ctx context.Context, organizationID string) ([]Network, error) {
	return list[Network](ctx, c, opNetworks, organizationID, nil)
}

// ListDevices returns the devices assigned to the organization's networks.
func (c *Client) ListDevices(ctx context.Context, organizationID string) ([]Device, error) {
	return list[Device](ctx, c, opDevices, organizationID, nil)
}

// ListDeviceAvailabilities returns the provider status of the organization's
// devices.
func (c *Client) ListDeviceAvailabilities(ctx context.Context, organizationID string) ([]DeviceAvailability, error) {
	return list[DeviceAvailability](ctx, c, opDeviceAvailabilities, organizationID, nil)
}

// ListSwitchPortUsageHistory returns the port usage of every switch of the
// organization during [start, end]. Meraki derives the interval length from the
// window; callers check the intervals they get.
func (c *Client) ListSwitchPortUsageHistory(
	ctx context.Context,
	organizationID string,
	start, end time.Time,
) ([]SwitchPortUsageHistory, error) {
	return list[SwitchPortUsageHistory](ctx, c, opSwitchPortsUsageHistory, organizationID, &window{start, end})
}

// ListSwitchPortPackets returns the packet statistics of every port of the
// switch over the last timespan.
func (c *Client) ListSwitchPortPackets(
	ctx context.Context,
	serial string,
	timespan time.Duration,
) ([]SwitchPortPackets, error) {
	query := url.Values{
		"timespan": {strconv.FormatInt(int64(timespan/time.Second), 10)},
	}
	return get[SwitchPortPackets](ctx, c, opSwitchPortsPackets, serial, query)
}

// ListApplianceUplinkUsage returns the bytes the uplinks of every MX and Z
// network of the organization moved during [start, end].
func (c *Client) ListApplianceUplinkUsage(
	ctx context.Context,
	organizationID string,
	start, end time.Time,
) ([]ApplianceUplinkUsage, error) {
	return get[ApplianceUplinkUsage](ctx, c, opApplianceUplinksUsage, organizationID, window{start, end}.query())
}

// ListApplianceUplinkStatuses returns the reported uplink state of each MX,
// Z-series appliance and Secure Router in the organization.
func (c *Client) ListApplianceUplinkStatuses(ctx context.Context, organizationID string) ([]ApplianceUplinkStatus, error) {
	return list[ApplianceUplinkStatus](ctx, c, opApplianceUplinkStatuses, organizationID, nil)
}

// ListUplinkLossAndLatency returns MX loss and latency samples by uplink and
// destination during [start, end]. Meraki permits at most five minutes, ending
// at least two minutes in the past. This organization operation is unpaginated.
func (c *Client) ListUplinkLossAndLatency(
	ctx context.Context,
	organizationID string,
	start, end time.Time,
) ([]UplinkLossAndLatency, error) {
	return get[UplinkLossAndLatency](ctx, c, opUplinkLossAndLatency, organizationID, window{start, end}.query())
}

// ProbeOrganization checks access to the organization by reading one minimal
// assigned-device page. It ignores pagination and returns no inventory. Listing
// organizations cannot prove access: Meraki exempts that call from the
// organization's API source-IP restrictions.
func (c *Client) ProbeOrganization(ctx context.Context, organizationID string) error {
	op := opDevices
	op.pageSize = minPageSize
	target, err := c.operationURL(op, organizationID, nil)
	if err != nil {
		return err
	}
	_, err = fetchPage[Device](ctx, c, target, op)
	return err
}

// window is the [t0, t1] time range of a request.
type window struct {
	start, end time.Time
}

func (w window) query() url.Values {
	return url.Values{
		"t0": {w.start.UTC().Format(time.RFC3339Nano)},
		"t1": {w.end.UTC().Format(time.RFC3339Nano)},
	}
}

// operationURL builds the first request of op. id fills the path parameter, and
// query is sent with perPage when op is paginated.
func (c *Client) operationURL(op operation, id string, query url.Values) (*url.URL, error) {
	path := op.path
	if before, rest, ok := strings.Cut(path, "{"); ok {
		param, after, _ := strings.Cut(rest, "}")
		// PathEscape keeps dot segments, which would climb out of the collection.
		if id == "" || id == "." || id == ".." {
			return nil, fmt.Errorf("%s: invalid %s", op.name, param)
		}
		path = before + url.PathEscape(id) + after
	}
	target, err := url.Parse(c.baseURL.String() + path)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid operation path", op.name)
	}
	q := make(url.Values)
	maps.Copy(q, query)
	if op.pageSize > 0 {
		q.Set("perPage", strconv.Itoa(op.pageSize))
	}
	target.RawQuery = q.Encode()
	return target, nil
}

// list follows rel="next" links to the last page. It returns the complete list,
// or nil and an error: a failed later page never looks like a shorter list. The
// pages are not an atomic snapshot; the provider can change data between them.
// A non-nil win is sent with every page.
func list[T any](ctx context.Context, c *Client, op operation, id string, win *window) ([]T, error) {
	var query url.Values
	if win != nil {
		query = win.query()
	}
	target, err := c.operationURL(op, id, query)
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
		var key string
		target, key, err = c.pageURL(target, operationPath, op.maxPageSize, win)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid pagination target", op.name)
		}
		if visited[key] {
			return nil, fmt.Errorf("%s: cyclic pagination", op.name)
		}
		visited[key] = true

		p, err := fetchPage[T](ctx, c, target, op)
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

// get reads an unpaginated operation.
func get[T any](ctx context.Context, c *Client, op operation, id string, query url.Values) ([]T, error) {
	target, err := c.operationURL(op, id, query)
	if err != nil {
		return nil, err
	}
	p, err := fetchPage[T](ctx, c, target, op)
	if err != nil {
		return nil, err
	}
	return p.items, nil
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

// fetchPage sends one authenticated GET and decodes the array of op's response.
func fetchPage[T any](ctx context.Context, c *Client, target *url.URL, op operation) (page[T], error) {
	req, err := c.newRequest(ctx, target)
	if err != nil {
		return page[T]{}, safeError(op.name, "cannot prepare authenticated request", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return page[T]{}, transportError(op.name, err)
	}
	// Clients built by web.NewHTTPClient already bound bodies; this keeps the
	// bound for any other http.Client.
	resp.Body = web.LimitResponseBody(resp.Body, web.HTTPClientResponseBodyLimit(c.httpClient))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return page[T]{}, newHTTPError(op.name, resp)
	}
	// net/http decompresses gzip itself; any encoding still present is unreadable.
	if enc := strings.TrimSpace(resp.Header.Get("Content-Encoding")); enc != "" && !strings.EqualFold(enc, "identity") {
		return page[T]{}, fmt.Errorf("%s: unsupported response encoding", op.name)
	}
	decode := decodeArray[T]
	if op.itemsObject {
		decode = decodeItems[T]
	}
	items, err := decode(resp.Body, op.name)
	if err != nil {
		return page[T]{}, err
	}
	if err := ctx.Err(); err != nil {
		return page[T]{}, safeError(op.name, "request canceled", err)
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

// decodeItems requires exactly one JSON object whose "items" member is an array
// decodeArray accepts. Other members, such as "meta", are ignored.
func decodeItems[T any](r io.Reader, opName string) ([]T, error) {
	dec := json.NewDecoder(r)
	var object *struct {
		Items json.RawMessage `json:"items"`
	}
	if err := dec.Decode(&object); err != nil {
		return nil, safeError(opName, "invalid JSON object response", err)
	}
	if object == nil || object.Items == nil {
		return nil, fmt.Errorf("%s: expected a JSON object with items", opName)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, safeError(opName, "unexpected trailing response data", err)
	}
	return decodeArray[T](bytes.NewReader(object.Items), opName)
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
