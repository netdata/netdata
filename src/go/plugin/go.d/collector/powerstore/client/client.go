// SPDX-License-Identifier: GPL-3.0-or-later

package client

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path"
	"strconv"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

const (
	apiBasePath  = "/api/rest"
	dellEMCToken = "DELL-EMC-TOKEN"
	defaultLimit = "2000"
)

// New creates a new PowerStore REST API client.
func New(client web.ClientConfig, request web.RequestConfig) (*Client, error) {
	httpClient, err := web.NewHTTPClient(client)
	if err != nil {
		return nil, err
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("error creating cookie jar: %v", err)
	}
	httpClient.Jar = jar

	return &Client{
		Request:    request,
		httpClient: httpClient,
		csrf:       &csrfToken{},
	}, nil
}

// Client represents a Dell PowerStore REST API client.
type Client struct {
	Request    web.RequestConfig
	httpClient *http.Client
	csrf       *csrfToken
}

// Login authenticates with the PowerStore API.
// GET /api/rest/login_session with Basic Auth.
// Caches auth_cookie (via cookiejar) and DELL-EMC-TOKEN (from response header).
func (c *Client) Login() error {
	req := c.createRequest("/login_session")

	resp, err := c.doOK(req)
	defer web.CloseBody(resp)
	if err != nil {
		return fmt.Errorf("login failed: %v", err)
	}

	c.cacheCSRFToken(resp)
	return nil
}

// Logout clears the client session state.
func (c *Client) Logout() {
	c.csrf.unset()
}

// Clusters returns all clusters.
func (c *Client) Clusters() ([]Cluster, error) {
	return doGetAllPages[Cluster](c, "/cluster", nil)
}

// Appliances returns all appliances.
func (c *Client) Appliances() ([]Appliance, error) {
	return doGetAllPages[Appliance](c, "/appliance", nil)
}

// Volumes returns all volumes.
func (c *Client) Volumes() ([]Volume, error) {
	return doGetAllPages[Volume](c, "/volume", nil)
}

// AllHardware returns all hardware components.
func (c *Client) AllHardware() ([]Hardware, error) {
	return doGetAllPages[Hardware](c, "/hardware", nil)
}

// Alerts returns alerts filtered by state.
func (c *Client) Alerts(state string) ([]Alert, error) {
	return doGetAllPages[Alert](c, "/alert", url.Values{"state": {"eq." + state}})
}

// FcPorts returns all Fibre Channel ports.
func (c *Client) FcPorts() ([]FcPort, error) {
	return doGetAllPages[FcPort](c, "/fc_port", nil)
}

// EthPorts returns all Ethernet ports.
func (c *Client) EthPorts() ([]EthPort, error) {
	return doGetAllPages[EthPort](c, "/eth_port", nil)
}

// FileSystems returns all file systems.
func (c *Client) FileSystems() ([]FileSystem, error) {
	return doGetAllPages[FileSystem](c, "/file_system", nil)
}

// NASServers returns all NAS servers.
func (c *Client) NASServers() ([]NAS, error) {
	return doGetAllPages[NAS](c, "/nas_server", nil)
}

// PerformanceMetricsByAppliance returns performance metrics for an appliance.
func (c *Client) PerformanceMetricsByAppliance(id string) ([]PerformanceMetrics, error) {
	return doMetrics[PerformanceMetrics](c, "performance_metrics_by_appliance", id, "Five_Mins")
}

// PerformanceMetricsByVolume returns performance metrics for a volume.
func (c *Client) PerformanceMetricsByVolume(id string) ([]PerformanceMetrics, error) {
	return doMetrics[PerformanceMetrics](c, "performance_metrics_by_volume", id, "Five_Mins")
}

// PerformanceMetricsByNode returns performance metrics for a node.
func (c *Client) PerformanceMetricsByNode(id string) ([]PerformanceMetrics, error) {
	return doMetrics[PerformanceMetrics](c, "performance_metrics_by_node", id, "Five_Mins")
}

// PerformanceMetricsByFcPort returns performance metrics for an FC port.
func (c *Client) PerformanceMetricsByFcPort(id string) ([]PerformanceMetrics, error) {
	return doMetrics[PerformanceMetrics](c, "performance_metrics_by_fe_fc_port", id, "Five_Mins")
}

// EthPortPerformanceMetrics returns performance metrics for an Ethernet port.
func (c *Client) EthPortPerformanceMetrics(id string) ([]EthPortMetrics, error) {
	return doMetrics[EthPortMetrics](c, "performance_metrics_by_fe_eth_port", id, "Five_Mins")
}

// PerformanceMetricsByFileSystem returns performance metrics for a file system.
func (c *Client) PerformanceMetricsByFileSystem(id string) ([]FileSystemMetrics, error) {
	return doMetrics[FileSystemMetrics](c, "performance_metrics_by_file_system", id, "Five_Mins")
}

// SpaceMetricsByCluster returns space metrics for a cluster.
func (c *Client) SpaceMetricsByCluster(id string) ([]SpaceMetrics, error) {
	return doMetrics[SpaceMetrics](c, "space_metrics_by_cluster", id, "One_Day")
}

// SpaceMetricsByAppliance returns space metrics for an appliance.
func (c *Client) SpaceMetricsByAppliance(id string) ([]SpaceMetrics, error) {
	return doMetrics[SpaceMetrics](c, "space_metrics_by_appliance", id, "One_Day")
}

// SpaceMetricsByVolume returns space metrics for a volume.
func (c *Client) SpaceMetricsByVolume(id string) ([]SpaceMetrics, error) {
	return doMetrics[SpaceMetrics](c, "space_metrics_by_volume", id, "Five_Mins")
}

// WearMetricsByDrive returns wear metrics for a drive.
func (c *Client) WearMetricsByDrive(id string) ([]WearMetrics, error) {
	return doMetrics[WearMetrics](c, "wear_metrics_by_drive", id, "Five_Mins")
}

// CopyMetricsByAppliance returns copy/replication metrics for an appliance.
func (c *Client) CopyMetricsByAppliance(id string) ([]CopyMetrics, error) {
	return doMetrics[CopyMetrics](c, "copy_metrics_by_appliance", id, "Five_Mins")
}

func doMetrics[T any](c *Client, entity, entityID, interval string) ([]T, error) {
	body := MetricsRequest{Entity: entity, EntityID: entityID, Interval: interval}
	var v []T
	if err := c.doPostWithRetry(&v, "/metrics/generate", body); err != nil {
		return nil, err
	}
	return v, nil
}

func (c *Client) createRequest(urlPath string) web.RequestConfig {
	req := c.Request.Copy()
	u, _ := url.Parse(req.URL)
	u.Path = path.Join(u.Path, apiBasePath, urlPath)
	req.URL = u.String()
	return req
}

func (c *Client) createGetRequest(urlPath string, params url.Values) web.RequestConfig {
	req := c.createRequest(urlPath)
	u, _ := url.Parse(req.URL)
	q := u.Query()
	q.Set("select", "*")
	q.Set("limit", defaultLimit)
	for k, vals := range params {
		for _, v := range vals {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	req.URL = u.String()

	if tok := c.csrf.get(); tok != "" {
		if req.Headers == nil {
			req.Headers = make(map[string]string)
		}
		req.Headers[dellEMCToken] = tok
	}
	return req
}

func (c *Client) createPostRequest(urlPath string, body any) (web.RequestConfig, error) {
	req := c.createRequest(urlPath)

	b, err := json.Marshal(body)
	if err != nil {
		return req, fmt.Errorf("error marshaling request body: %v", err)
	}

	if req.Headers == nil {
		req.Headers = make(map[string]string)
	}
	req.Headers["Content-Type"] = "application/json"
	if tok := c.csrf.get(); tok != "" {
		req.Headers[dellEMCToken] = tok
	}
	req.Method = http.MethodPost
	req.Body = string(b)
	return req, nil
}

func (c *Client) do(req web.RequestConfig) (*http.Response, error) {
	httpReq, err := web.NewHTTPRequest(req)
	if err != nil {
		return nil, fmt.Errorf("error creating http request to %s: %v", req.URL, err)
	}
	return c.httpClient.Do(httpReq)
}

func (c *Client) doOK(req web.RequestConfig) (*http.Response, error) {
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	if err = checkStatusCode(resp); err != nil {
		err = fmt.Errorf("%s returned %v", req.URL, err)
	}
	return resp, err
}

func (c *Client) doOKWithRetry(req web.RequestConfig) (*http.Response, error) {
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	// PowerStore returns 403 when the session/token is stale (not 401)
	if resp.StatusCode == http.StatusForbidden {
		web.CloseBody(resp)
		if err = c.Login(); err != nil {
			return nil, fmt.Errorf("re-login after 403 failed: %v", err)
		}
		req = c.applyCSRF(req)
		return c.doOK(req)
	}
	if err = checkStatusCode(resp); err != nil {
		err = fmt.Errorf("%s returned %v", req.URL, err)
	}
	return resp, err
}

// doGetAllPages fetches all pages from a paginated GET endpoint.
// PowerStore returns HTTP 206 (Partial Content) when more pages are available,
// with a server-enforced maximum of 2000 items per page.
func doGetAllPages[T any](c *Client, urlPath string, params url.Values) ([]T, error) {
	var all []T
	offset := 0

	for {
		reqParams := make(url.Values)
		maps.Copy(reqParams, params)
		if offset > 0 {
			reqParams.Set("offset", strconv.Itoa(offset))
		}

		req := c.createGetRequest(urlPath, reqParams)

		resp, err := c.doOKWithRetry(req)
		if err != nil {
			web.CloseBody(resp)
			return nil, err
		}

		var page []T
		err = json.NewDecoder(resp.Body).Decode(&page)
		c.cacheCSRFToken(resp)
		partial := resp.StatusCode == http.StatusPartialContent
		web.CloseBody(resp)

		if err != nil {
			return nil, fmt.Errorf("error decoding %s response: %v", urlPath, err)
		}

		all = append(all, page...)

		if !partial || len(page) == 0 {
			break
		}
		offset += len(page)
	}

	return all, nil
}

func (c *Client) doPostWithRetry(dst any, urlPath string, body any) error {
	req, err := c.createPostRequest(urlPath, body)
	if err != nil {
		return err
	}
	resp, err := c.doOKWithRetry(req)
	defer web.CloseBody(resp)
	if err != nil {
		return err
	}
	c.cacheCSRFToken(resp)
	return json.NewDecoder(resp.Body).Decode(dst)
}

func (c *Client) cacheCSRFToken(resp *http.Response) {
	if resp == nil {
		return
	}
	if tok := resp.Header.Get(dellEMCToken); tok != "" {
		c.csrf.set(tok)
	}
}

func (c *Client) applyCSRF(req web.RequestConfig) web.RequestConfig {
	if tok := c.csrf.get(); tok != "" {
		if req.Headers == nil {
			req.Headers = make(map[string]string)
		}
		req.Headers[dellEMCToken] = tok
	}
	return req
}

func checkStatusCode(resp *http.Response) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP status code %d", resp.StatusCode)
	}
	return nil
}

// csrfToken safely stores the DELL-EMC-TOKEN value.
type csrfToken struct {
	mux   sync.RWMutex
	value string
}

func (t *csrfToken) get() string  { t.mux.RLock(); defer t.mux.RUnlock(); return t.value }
func (t *csrfToken) set(v string) { t.mux.Lock(); defer t.mux.Unlock(); t.value = v }
func (t *csrfToken) unset()       { t.set("") }
