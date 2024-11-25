// SPDX-License-Identifier: GPL-3.0-or-later

package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

/*
The REST API is served from the VxFlex OS Gateway.
The FxFlex Gateway connects to a single MDM and serves requests by querying the MDM
and reformatting the answers it receives from the MDM in s RESTful manner, back to a REST API.
The Gateway is stateless. It requires the MDM username and password for the login requests.
The login returns a token in the response, that is used for later authentication for other requests.

The token is valid for 8 hours from the time it was created, unless there has been no activity
for 10 minutes, of if the client has sent a logout request.

General URI:
- /api/login
- /api/logout
- /api/version
- /api/instances/                                              // GET all instances
- /api/types/{type}/instances                                  // POST (create) / GET all objects for a given type
- /api/instances/{type::id}                                    // GET by ID
- /api/instances/{type::id}/relationships/{Relationship name}  // GET
- /api/instances/querySelectedStatistics                       // POST Query selected statistics
- /api/instances/{type::id}/action/{actionName}                // POST a special action on an object
- /api/types/{type}/instances/action/{actionName}              // POST a special action on a given type

Types:
- System
- Sds
- StoragePool
- ProtectionDomain
- Device
- Volume
- VTree
- Sdc
- User
- FaultSet
- RfcacheDevice
- Alerts

Actions:
- querySelectedStatistics      // All types except Alarm and User
- querySystemLimits            // System
- queryDisconnectedSdss        // Sds
- querySdsNetworkLatencyMeters // Sds
- queryFailedDevices"          // Device. Note: works strange!

Relationships:
- Statistics           // All types except Alarm and User
- ProtectionDomain // System
- Sdc              // System
- User             // System
- StoragePool      // ProtectionDomain
- FaultSet         // ProtectionDomain
- Sds              // ProtectionDomain
- RfcacheDevice    // Sds
- Device           // Sds, StoragePool
- Volume           // Sdc, StoragePool
- VTree            // StoragePool
*/

// New creates new ScaleIO client.
func New(client web.ClientConfig, request web.RequestConfig) (*Client, error) {
	httpClient, err := web.NewHTTPClient(client)
	if err != nil {
		return nil, err
	}
	return &Client{
		Request:    request,
		httpClient: httpClient,
		token:      newToken(),
	}, nil
}

// Client represents ScaleIO client.
type Client struct {
	Request    web.RequestConfig
	httpClient *http.Client
	token      *token
}

// LoggedIn reports whether the client is logged in.
func (c *Client) LoggedIn() bool {
	return c.token.isSet()
}

// Login connects to FxFlex Gateway to get the token that is used for later authentication for other requests.
func (c *Client) Login() error {
	if c.LoggedIn() {
		_ = c.Logout()
	}
	req := c.createLoginRequest()
	resp, err := c.doOK(req)
	defer web.CloseBody(resp)
	if err != nil {
		return err
	}

	token, err := decodeToken(resp.Body)
	if err != nil {
		return err
	}

	c.token.set(token)
	return nil
}

// Logout sends logout request and unsets token.
func (c *Client) Logout() error {
	if !c.LoggedIn() {
		return nil
	}
	req := c.createLogoutRequest()
	c.token.unset()

	resp, err := c.do(req)
	defer web.CloseBody(resp)
	return err
}

// APIVersion returns FxFlex Gateway API version.
func (c *Client) APIVersion() (Version, error) {
	req := c.createAPIVersionRequest()
	resp, err := c.doOK(req)
	defer web.CloseBody(resp)
	if err != nil {
		return Version{}, err
	}
	return decodeVersion(resp.Body)
}

// SelectedStatistics returns selected statistics.
func (c *Client) SelectedStatistics(query SelectedStatisticsQuery) (SelectedStatistics, error) {
	b, _ := json.Marshal(query)
	req := c.createSelectedStatisticsRequest(b)
	var stats SelectedStatistics
	err := c.doJSONWithRetry(&stats, req)
	return stats, err
}

// Instances returns all instances.
func (c *Client) Instances() (Instances, error) {
	req := c.createInstancesRequest()
	var instances Instances
	err := c.doJSONWithRetry(&instances, req)
	return instances, err
}

func (c *Client) createLoginRequest() web.RequestConfig {
	req := c.Request.Copy()
	u, _ := url.Parse(req.URL)
	u.Path = path.Join(u.Path, "/api/login")
	req.URL = u.String()
	return req
}

func (c *Client) createLogoutRequest() web.RequestConfig {
	req := c.Request.Copy()
	u, _ := url.Parse(req.URL)
	u.Path = path.Join(u.Path, "/api/logout")
	req.URL = u.String()
	req.Password = c.token.get()
	return req
}

func (c *Client) createAPIVersionRequest() web.RequestConfig {
	req := c.Request.Copy()
	u, _ := url.Parse(req.URL)
	u.Path = path.Join(u.Path, "/api/version")
	req.URL = u.String()
	req.Password = c.token.get()
	return req
}

func (c *Client) createSelectedStatisticsRequest(query []byte) web.RequestConfig {
	req := c.Request.Copy()
	u, _ := url.Parse(req.URL)
	u.Path = path.Join(u.Path, "/api/instances/querySelectedStatistics")
	req.URL = u.String()
	req.Password = c.token.get()
	req.Method = http.MethodPost
	req.Headers = map[string]string{
		"Content-Type": "application/json",
	}
	req.Body = string(query)
	return req
}

func (c *Client) createInstancesRequest() web.RequestConfig {
	req := c.Request.Copy()
	u, _ := url.Parse(req.URL)
	u.Path = path.Join(u.Path, "/api/instances")
	req.URL = u.String()
	req.Password = c.token.get()
	return req
}

func (c *Client) do(req web.RequestConfig) (*http.Response, error) {
	httpReq, err := web.NewHTTPRequest(req)
	if err != nil {
		return nil, fmt.Errorf("error on creating http request to %s: %v", req.URL, err)
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
	if resp.StatusCode == http.StatusUnauthorized {
		if err = c.Login(); err != nil {
			return resp, err
		}
		req.Password = c.token.get()
		return c.doOK(req)
	}
	if err = checkStatusCode(resp); err != nil {
		err = fmt.Errorf("%s returned %v", req.URL, err)
	}
	return resp, err
}

func (c *Client) doJSONWithRetry(dst any, req web.RequestConfig) error {
	resp, err := c.doOKWithRetry(req)
	defer web.CloseBody(resp)
	if err != nil {
		return err
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

func checkStatusCode(resp *http.Response) error {
	// For all 4xx and 5xx return codes, the body may contain an apiError
	// instance with more specifics about the failure.
	if resp.StatusCode >= 400 {
		e := error(&apiError{})
		if err := json.NewDecoder(resp.Body).Decode(e); err != nil {
			e = err
		}
		return fmt.Errorf("HTTP status code %d : %v", resp.StatusCode, e)
	}

	// 200(OK), 201(Created), 202(Accepted), 204 (No Content).
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("HTTP status code %d", resp.StatusCode)
	}
	return nil
}

func decodeVersion(reader io.Reader) (ver Version, err error) {
	bs, err := io.ReadAll(reader)
	if err != nil {
		return ver, err
	}
	parts := strings.Split(strings.Trim(string(bs), "\n "), ".")
	if len(parts) != 2 {
		return ver, fmt.Errorf("can't parse: %s", string(bs))
	}
	if ver.Major, err = strconv.ParseInt(parts[0], 10, 64); err != nil {
		return ver, err
	}
	ver.Minor, err = strconv.ParseInt(parts[1], 10, 64)
	return ver, err
}

func decodeToken(reader io.Reader) (string, error) {
	bs, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return strings.Trim(string(bs), `"`), nil
}

type token struct {
	mux   *sync.RWMutex
	value string
}

func newToken() *token        { return &token{mux: &sync.RWMutex{}} }
func (t *token) get() string  { t.mux.RLock(); defer t.mux.RUnlock(); return t.value }
func (t *token) set(v string) { t.mux.Lock(); defer t.mux.Unlock(); t.value = v }
func (t *token) unset()       { t.set("") }
func (t *token) isSet() bool  { return t.get() != "" }
