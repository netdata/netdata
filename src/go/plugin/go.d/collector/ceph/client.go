// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

const (
	activeDiscoveryMaxHops = 5
	urlPathAPIClusterFSID  = "/api/health/get_cluster_fsid"
)

type apiHTTPError struct {
	operation string
	status    int
	redirect  bool
}

func (e *apiHTTPError) Error() string {
	return fmt.Sprintf("%s returned HTTP status %d", e.operation, e.status)
}

type apiTransportError struct {
	operation string
	err       error
}

func (e *apiTransportError) Error() string {
	return fmt.Sprintf("%s request failed: %v", e.operation, e.err)
}

func (e *apiTransportError) Unwrap() error { return e.err }

type cephClient struct {
	httpClient             *http.Client
	requestConfig          web.RequestConfig
	configuredBase         *url.URL
	notFollowRedirects     bool
	allowedRedirectOrigins map[string]struct{}

	username        string
	password        string
	bearerTokenFile string

	discoveryMu sync.Mutex
	authMu      sync.Mutex
	stateMu     sync.RWMutex
	activeBase  *url.URL
	jwt         string
}

func newCephClient(httpClient *http.Client, cfg web.RequestConfig, notFollowRedirects bool, allowedRedirectOrigins []string) (*cephClient, error) {
	if httpClient == nil {
		return nil, errors.New("HTTP client is nil")
	}

	base, err := parseDashboardBaseURL(cfg.URL)
	if err != nil {
		return nil, err
	}
	trustedOrigins, err := parseAllowedRedirectOrigins(base, allowedRedirectOrigins)
	if err != nil {
		return nil, err
	}
	for name := range cfg.Headers {
		switch strings.ToLower(name) {
		case "authorization", "cookie", "host":
			return nil, fmt.Errorf("header %q is managed by the Ceph client", name)
		}
	}

	c := &cephClient{
		httpClient:             httpClient,
		requestConfig:          cfg.Copy(),
		configuredBase:         base,
		notFollowRedirects:     notFollowRedirects,
		allowedRedirectOrigins: trustedOrigins,
		username:               cfg.Username,
		password:               cfg.Password,
		bearerTokenFile:        cfg.BearerTokenFile,
	}

	// Dashboard redirects identify the active MGR but do not preserve API paths.
	// Always inspect them here instead of delegating them to net/http.
	c.httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	// Ceph credentials are not generic HTTP Basic credentials. They are sent only
	// in the JSON login body, or as a bearer token after active-MGR discovery.
	c.requestConfig.Username = ""
	c.requestConfig.Password = ""
	c.requestConfig.BearerTokenFile = ""
	c.requestConfig.Method = ""
	c.requestConfig.Body = ""

	return c, nil
}

func parseDashboardBaseURL(rawURL string) (*url.URL, error) {
	if rawURL == "" {
		return nil, errors.New("URL is required but not set")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("URL scheme must be http or https")
	}
	if u.Host == "" {
		return nil, errors.New("URL host is required")
	}
	if u.User != nil {
		return nil, errors.New("URL userinfo is not allowed")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("URL query and fragment are not allowed")
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	if u.Path == "." {
		u.Path = ""
	}
	u.RawPath = ""
	return u, nil
}

func (c *cephClient) getJSON(ctx context.Context, operation, endpoint, accept string, query url.Values, dst any) error {
	_, err := c.doJSON(ctx, operation, http.MethodGet, endpoint, accept, query, nil, dst)
	return err
}

func (c *cephClient) getJSONWithHeaders(ctx context.Context, operation, endpoint, accept string, query url.Values, dst any) (http.Header, error) {
	return c.doJSON(ctx, operation, http.MethodGet, endpoint, accept, query, nil, dst)
}

func (c *cephClient) doJSON(ctx context.Context, operation, method, endpoint, accept string, query url.Values, body []byte, dst any) (http.Header, error) {
	var retriedAuth, retriedRedirect, retriedTransport bool

	for attempts := 0; attempts < 4; attempts++ {
		base, err := c.ensureActiveBase(ctx)
		if err != nil {
			return nil, err
		}

		token, managed, err := c.tokenForRequest(ctx, base)
		if err != nil {
			var httpErr *apiHTTPError
			if errors.As(err, &httpErr) && httpErr.redirect && !retriedRedirect && !c.notFollowRedirects {
				retriedRedirect = true
				c.invalidateActive(base)
				continue
			}
			var transportErr *apiTransportError
			if errors.As(err, &transportErr) && method == http.MethodGet && !retriedTransport && ctx.Err() == nil {
				retriedTransport = true
				c.invalidateActive(base)
				continue
			}
			return nil, err
		}

		resp, err := c.request(ctx, base, method, endpoint, accept, query, body, token)
		if err != nil {
			if method == http.MethodGet && !retriedTransport && ctx.Err() == nil {
				retriedTransport = true
				c.invalidateActive(base)
				continue
			}
			return nil, fmt.Errorf("%s request failed: %w", operation, err)
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			headers := resp.Header.Clone()
			defer web.CloseBody(resp)
			if dst != nil {
				decoder := json.NewDecoder(resp.Body)
				decoder.UseNumber()
				if err := decoder.Decode(dst); err != nil {
					return nil, fmt.Errorf("decode %s response: %w", operation, err)
				}
			}
			return headers, nil
		}

		status := resp.StatusCode
		isRedirect := isRedirectStatus(status)
		web.CloseBody(resp)

		if status == http.StatusUnauthorized && managed && !retriedAuth {
			retriedAuth = true
			c.invalidateJWT(token)
			continue
		}
		if isRedirect && !retriedRedirect && !c.notFollowRedirects {
			retriedRedirect = true
			c.invalidateActive(base)
			continue
		}
		return nil, &apiHTTPError{operation: operation, status: status, redirect: isRedirect}
	}

	return nil, fmt.Errorf("%s exceeded bounded authentication/failover retries", operation)
}

func (c *cephClient) ensureActiveBase(ctx context.Context) (*url.URL, error) {
	c.stateMu.RLock()
	active := cloneURL(c.activeBase)
	c.stateMu.RUnlock()
	if active != nil {
		return active, nil
	}

	c.discoveryMu.Lock()
	defer c.discoveryMu.Unlock()

	c.stateMu.RLock()
	active = cloneURL(c.activeBase)
	c.stateMu.RUnlock()
	if active != nil {
		return active, nil
	}

	active, err := c.discoverActiveBase(ctx)
	if err != nil {
		return nil, err
	}
	c.stateMu.Lock()
	c.activeBase = cloneURL(active)
	c.stateMu.Unlock()
	return active, nil
}

func (c *cephClient) discoverActiveBase(ctx context.Context) (*url.URL, error) {
	base := cloneURL(c.configuredBase)
	visited := make(map[string]bool, activeDiscoveryMaxHops+1)

	for hop := 0; hop <= activeDiscoveryMaxHops; hop++ {
		key := base.String()
		if visited[key] {
			return nil, errors.New("active-MGR redirect loop detected")
		}
		visited[key] = true

		resp, err := c.request(ctx, base, http.MethodGet, urlPathAPIClusterFSID, hdrAcceptVersion, nil, nil, "")
		if err != nil {
			return nil, fmt.Errorf("active-MGR discovery failed: %w", err)
		}
		status := resp.StatusCode
		location := resp.Header.Get("Location")
		web.CloseBody(resp)

		switch {
		case status >= 200 && status < 300,
			status == http.StatusUnauthorized,
			status == http.StatusForbidden:
			return base, nil
		case isRedirectStatus(status):
			if c.notFollowRedirects {
				return nil, fmt.Errorf("active-MGR discovery redirect rejected by not_follow_redirects")
			}
			if hop == activeDiscoveryMaxHops {
				return nil, fmt.Errorf("active-MGR discovery exceeded %d redirects", activeDiscoveryMaxHops)
			}
			next, err := redirectedDashboardBase(base, location, urlPathAPIClusterFSID)
			if err != nil {
				return nil, err
			}
			if _, ok := c.allowedRedirectOrigins[dashboardOrigin(next)]; !ok {
				return nil, errors.New("active-MGR redirect origin is not trusted; add it to allowed_redirect_origins")
			}
			base = next
		default:
			return nil, &apiHTTPError{operation: "active-MGR discovery", status: status}
		}
	}

	return nil, errors.New("active-MGR discovery failed")
}

func parseAllowedRedirectOrigins(configuredBase *url.URL, values []string) (map[string]struct{}, error) {
	result := map[string]struct{}{dashboardOrigin(configuredBase): {}}
	for _, value := range values {
		origin, err := parseDashboardBaseURL(value)
		if err != nil {
			return nil, fmt.Errorf("invalid allowed_redirect_origins entry: %w", err)
		}
		if origin.Path != "" {
			return nil, errors.New("allowed_redirect_origins entries must not contain a path")
		}
		result[dashboardOrigin(origin)] = struct{}{}
	}
	return result, nil
}

func dashboardOrigin(value *url.URL) string {
	scheme := strings.ToLower(value.Scheme)
	host := strings.ToLower(value.Hostname())
	port := value.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" {
		host += ":" + port
	}
	return (&url.URL{Scheme: scheme, Host: host}).String()
}

func redirectedDashboardBase(current *url.URL, location, probePath string) (*url.URL, error) {
	if location == "" {
		return nil, errors.New("active-MGR redirect has no Location header")
	}
	ref, err := url.Parse(location)
	if err != nil {
		return nil, fmt.Errorf("parse active-MGR redirect: %w", err)
	}
	next := current.ResolveReference(ref)
	if next.Scheme != "http" && next.Scheme != "https" {
		return nil, fmt.Errorf("active-MGR redirect scheme must be http or https")
	}
	if next.Host == "" {
		return nil, errors.New("active-MGR redirect host is required")
	}
	if next.User != nil {
		return nil, errors.New("active-MGR redirect userinfo is not allowed")
	}
	if current.Scheme == "https" && next.Scheme == "http" {
		return nil, errors.New("active-MGR redirect cannot downgrade HTTPS to HTTP")
	}

	redirectPath := strings.TrimSuffix(next.Path, "/")
	probe := strings.TrimSuffix(probePath, "/")
	if strings.HasSuffix(redirectPath, probe) {
		redirectPath = strings.TrimSuffix(strings.TrimSuffix(redirectPath, probe), "/")
	}
	if redirectPath == "." || redirectPath == "/" {
		redirectPath = ""
	}
	next.Path = redirectPath
	next.RawPath = ""
	next.RawQuery = ""
	next.Fragment = ""
	return next, nil
}

func (c *cephClient) tokenForRequest(ctx context.Context, base *url.URL) (token string, managed bool, err error) {
	if c.bearerTokenFile != "" {
		bs, err := os.ReadFile(c.bearerTokenFile)
		if err != nil {
			return "", false, fmt.Errorf("read bearer token file: %w", err)
		}
		token := strings.TrimSpace(string(bs))
		if token == "" {
			return "", false, errors.New("bearer token file is empty")
		}
		return token, false, nil
	}

	c.stateMu.RLock()
	token = c.jwt
	c.stateMu.RUnlock()
	if token != "" {
		return token, true, nil
	}

	c.authMu.Lock()
	defer c.authMu.Unlock()
	c.stateMu.RLock()
	token = c.jwt
	c.stateMu.RUnlock()
	if token != "" {
		return token, true, nil
	}

	var response authLoginResp
	credentials := struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{Username: c.username, Password: c.password}
	body, err := json.Marshal(credentials)
	if err != nil {
		return "", true, fmt.Errorf("encode login request: %w", err)
	}
	resp, err := c.request(ctx, base, http.MethodPost, urlPathApiAuth, hdrAcceptVersion, nil, body, "")
	if err != nil {
		return "", true, &apiTransportError{operation: "Dashboard login", err: err}
	}
	defer web.CloseBody(resp)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", true, &apiHTTPError{
			operation: "Dashboard login",
			status:    resp.StatusCode,
			redirect:  isRedirectStatus(resp.StatusCode),
		}
	}
	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		return "", true, fmt.Errorf("decode login response: %w", err)
	}
	if response.Token == "" {
		return "", true, errors.New("Dashboard login returned an empty token")
	}

	c.stateMu.Lock()
	c.jwt = response.Token
	c.stateMu.Unlock()
	return response.Token, true, nil
}

func (c *cephClient) request(ctx context.Context, base *url.URL, method, endpoint, accept string, query url.Values, body []byte, token string) (*http.Response, error) {
	cfg := c.requestConfig.Copy()
	u := cloneURL(base)
	rawPath := strings.TrimSuffix(base.EscapedPath(), "/") + "/" + strings.TrimLeft(endpoint, "/")
	decodedPath, err := url.PathUnescape(rawPath)
	if err != nil {
		return nil, fmt.Errorf("invalid API endpoint path: %w", err)
	}
	u.Path = decodedPath
	u.RawPath = rawPath
	if u.RawPath == (&url.URL{Path: u.Path}).EscapedPath() {
		u.RawPath = ""
	}
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}
	cfg.URL = u.String()
	cfg.Method = method
	if len(body) > 0 {
		cfg.Body = string(body)
	}

	req, err := web.NewHTTPRequest(cfg)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	if accept == "" {
		accept = hdrAcceptVersion
	}
	req.Header.Set("Accept", accept)
	if len(body) > 0 {
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
		req.Header.Set("Content-Type", hdrContentTypeJson)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return c.httpClient.Do(req)
}

func (c *cephClient) invalidateActive(active *url.URL) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.activeBase != nil && active != nil && c.activeBase.String() == active.String() {
		c.activeBase = nil
		c.jwt = ""
	}
}

func (c *cephClient) invalidateJWT(token string) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.jwt == token {
		c.jwt = ""
	}
}

func (c *cephClient) logout(ctx context.Context) error {
	if c.bearerTokenFile != "" {
		return nil
	}
	c.stateMu.RLock()
	base := cloneURL(c.activeBase)
	token := c.jwt
	c.stateMu.RUnlock()
	if base == nil || token == "" {
		return nil
	}

	resp, err := c.request(ctx, base, http.MethodPost, urlPathApiAuthLogout, hdrAcceptVersion, nil, []byte("{}"), token)
	c.invalidateJWT(token)
	if err != nil {
		return err
	}
	defer web.CloseBody(resp)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &apiHTTPError{operation: "Dashboard logout", status: resp.StatusCode}
	}
	return nil
}

func isRedirectStatus(status int) bool {
	switch status {
	case http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusSeeOther,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect:
		return true
	default:
		return false
	}
}

func cloneURL(u *url.URL) *url.URL {
	if u == nil {
		return nil
	}
	cloned := *u
	return &cloned
}
