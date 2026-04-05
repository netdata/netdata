// SPDX-License-Identifier: GPL-3.0-or-later

package client

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

// New creates a new PowerVault MCI REST API client.
func New(client web.ClientConfig, request web.RequestConfig, digest string) (*Client, error) {
	httpClient, err := web.NewHTTPClient(client)
	if err != nil {
		return nil, err
	}

	return &Client{
		request:    request,
		httpClient: httpClient,
		digest:     digest,
	}, nil
}

// Client represents a Dell PowerVault MCI API client.
type Client struct {
	request    web.RequestConfig
	httpClient *http.Client
	digest     string // "sha256" or "md5"

	mu         sync.Mutex // protects sessionKey reads/writes
	sessionKey string
	authMu     sync.Mutex // serializes re-authentication
}

// Login authenticates with the PowerVault API.
// Hashes "username_password" with SHA-256 or MD5, then GET /api/login/<hash>.
func (c *Client) Login() error {
	key, err := c.login()
	if err != nil {
		return err
	}
	c.setSessionKey(key)
	return nil
}

// login authenticates and returns the session key without publishing it.
func (c *Client) login() (string, error) {
	hash := c.authHash()

	req := c.newRequest("/api/login/" + hash)

	resp, err := c.doOK(req)
	defer web.CloseBody(resp)
	if err != nil {
		return "", fmt.Errorf("login failed: %v", err)
	}

	var result struct {
		Status []StatusResponse `json:"status"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("login: error decoding response: %v", err)
	}
	if len(result.Status) == 0 || result.Status[0].ResponseType != "Success" {
		if len(result.Status) > 0 {
			return "", fmt.Errorf("login: authentication failed: %s (rc=%d)", result.Status[0].Response, result.Status[0].ReturnCode)
		}
		return "", fmt.Errorf("login: authentication failed: empty status response")
	}

	return result.Status[0].Response, nil
}

func (c *Client) setSessionKey(key string) {
	c.mu.Lock()
	c.sessionKey = key
	c.mu.Unlock()
}

// SetLocale sets the CLI output to English to prevent locale-dependent parsing.
func (c *Client) SetLocale() error {
	req := c.newSessionRequest("/api/set/cli-parameters/locale/English")
	resp, err := c.doOK(req)
	web.CloseBody(resp)
	return err
}

// setLocale sets the CLI locale to English using the given session key.
func (c *Client) setLocale(sessionKey string) error {
	req := c.newRequest("/api/set/cli-parameters/locale/English")
	if sessionKey != "" {
		req.Headers["sessionKey"] = sessionKey
	}
	resp, err := c.doOK(req)
	web.CloseBody(resp)
	return err
}

// System returns system information.
func (c *Client) System() ([]SystemInfo, error) {
	return doShow[SystemInfo](c, "/api/show/system", "system")
}

// Controllers returns all controllers.
func (c *Client) Controllers() ([]Controller, error) {
	return doShow[Controller](c, "/api/show/controllers", "controllers")
}

// Drives returns all drives.
func (c *Client) Drives() ([]Drive, error) {
	return doShow[Drive](c, "/api/show/disks", "drives")
}

// Fans returns all fans.
func (c *Client) Fans() ([]Fan, error) {
	return doShow[Fan](c, "/api/show/fans", "fan")
}

// PowerSupplies returns all power supplies.
func (c *Client) PowerSupplies() ([]PowerSupply, error) {
	return doShow[PowerSupply](c, "/api/show/power-supplies", "power-supplies")
}

// Sensors returns all sensor readings.
func (c *Client) Sensors() ([]Sensor, error) {
	return doShow[Sensor](c, "/api/show/sensor-status", "sensors")
}

// FRUs returns all Field Replaceable Units.
func (c *Client) FRUs() ([]FRU, error) {
	return doShow[FRU](c, "/api/show/frus", "enclosure-fru")
}

// Volumes returns all volumes.
func (c *Client) Volumes() ([]Volume, error) {
	return doShow[Volume](c, "/api/show/volumes", "volumes")
}

// Pools returns all storage pools.
func (c *Client) Pools() ([]Pool, error) {
	return doShow[Pool](c, "/api/show/pools", "pools")
}

// Ports returns all host ports.
func (c *Client) Ports() ([]Port, error) {
	return doShow[Port](c, "/api/show/ports", "port")
}

// ControllerStatistics returns performance stats for all controllers.
func (c *Client) ControllerStatistics() ([]ControllerStats, error) {
	return doShow[ControllerStats](c, "/api/show/controller-statistics", "controller-statistics")
}

// VolumeStatistics returns performance stats for all volumes.
func (c *Client) VolumeStatistics() ([]VolumeStats, error) {
	return doShow[VolumeStats](c, "/api/show/volume-statistics", "volume-statistics")
}

// PortStatistics returns I/O stats for all host ports.
func (c *Client) PortStatistics() ([]PortStats, error) {
	return doShow[PortStats](c, "/api/show/host-port-statistics", "host-port-statistics")
}

// PhyStatistics returns SAS PHY error stats.
func (c *Client) PhyStatistics() ([]PhyStats, error) {
	return doShow[PhyStats](c, "/api/show/host-phy-statistics", "sas-host-phy-statistics")
}

// doShow fetches a /api/show/<command> endpoint and extracts the named array from the response.
// Re-authenticates once on 401 (session expiry), including locale reset.
func doShow[T any](c *Client, urlPath, key string) ([]T, error) {
	req := c.newSessionRequest(urlPath)

	resp, err := c.doOK(req)
	if err != nil && resp != nil && resp.StatusCode == http.StatusUnauthorized {
		if loginErr := c.reAuth(); loginErr != nil {
			return nil, fmt.Errorf("%s: session expired and re-auth failed: %v", urlPath, loginErr)
		}
		req = c.newSessionRequest(urlPath)
		resp, err = c.doOK(req)
	}
	defer web.CloseBody(resp)
	if err != nil {
		return nil, err
	}

	var raw map[string]json.RawMessage
	if err = json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("%s: error decoding response: %v", urlPath, err)
	}

	// Check for API-level errors in the status envelope.
	if statusData, ok := raw["status"]; ok {
		var statuses []StatusResponse
		if json.Unmarshal(statusData, &statuses) == nil {
			for _, s := range statuses {
				if s.ResponseType == "Error" {
					return nil, fmt.Errorf("%s: API error: %s (rc=%d)", urlPath, s.Response, s.ReturnCode)
				}
			}
		}
	}

	data, ok := raw[key]
	if !ok {
		return nil, nil
	}

	var result []T
	if err = json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("%s: error decoding %q: %v", urlPath, key, err)
	}
	return result, nil
}

// reAuth re-authenticates and restores session locale.
// Serialized so concurrent 401 retries don't race on session state.
// The new session key is published only after locale setup, preventing
// concurrent requests from seeing a session with non-English locale.
func (c *Client) reAuth() error {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	key, err := c.login()
	if err != nil {
		return err
	}
	if err := c.setLocale(key); err != nil {
		return err
	}
	c.setSessionKey(key)
	return nil
}

func (c *Client) authHash() string {
	cred := c.request.Username + "_" + c.request.Password
	if c.digest == "md5" {
		return fmt.Sprintf("%x", md5.Sum([]byte(cred)))
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(cred)))
}

func (c *Client) newRequest(urlPath string) web.RequestConfig {
	req := c.request.Copy()
	u, _ := url.Parse(req.URL)
	u.Path = path.Join(u.Path, urlPath)
	req.URL = u.String()
	// Clear basic auth — MCI API uses hash-based auth, not HTTP Basic.
	req.Username = ""
	req.Password = ""
	if req.Headers == nil {
		req.Headers = make(map[string]string)
	}
	req.Headers["datatype"] = "json"
	return req
}

func (c *Client) newSessionRequest(urlPath string) web.RequestConfig {
	req := c.newRequest(urlPath)

	c.mu.Lock()
	key := c.sessionKey
	c.mu.Unlock()

	if key != "" {
		req.Headers["sessionKey"] = key
	}
	return req
}

func (c *Client) doOK(req web.RequestConfig) (*http.Response, error) {
	httpReq, err := web.NewHTTPRequest(req)
	if err != nil {
		return nil, fmt.Errorf("error creating request to %s: %v", req.URL, err)
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return resp, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		web.CloseBody(resp)
		// Return resp (body closed) so callers can inspect StatusCode.
		return resp, fmt.Errorf("%s: HTTP %d", req.URL, resp.StatusCode)
	}
	return resp, nil
}
