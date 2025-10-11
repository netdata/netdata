// SPDX-License-Identifier: GPL-3.0-or-later

package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

//  Session: https://vmware.github.io/vsphere-automation-sdk-rest/vsphere/index.html#SVC_com.vmware.cis.session
//  Health: https://vmware.github.io/vsphere-automation-sdk-rest/vsphere/index.html#SVC_com.vmware.appliance.health

const (
	pathCISSession             = "/rest/com/vmware/cis/session"
	pathHealthSystem           = "/rest/appliance/health/system"
	pathHealthSwap             = "/rest/appliance/health/swap"
	pathHealthStorage          = "/rest/appliance/health/storage"
	pathHealthSoftwarePackager = "/rest/appliance/health/software-packages"
	pathHealthMem              = "/rest/appliance/health/mem"
	pathHealthLoad             = "/rest/appliance/health/load"
	pathHealthDatabaseStorage  = "/rest/appliance/health/database-storage"
	pathHealthApplMgmt         = "/rest/appliance/health/applmgmt"

	apiSessIDKey = "vmware-api-session-id"
)

type sessionToken struct {
	m  *sync.RWMutex
	id string
}

func (s *sessionToken) set(id string) {
	s.m.Lock()
	defer s.m.Unlock()
	s.id = id
}

func (s *sessionToken) get() string {
	s.m.RLock()
	defer s.m.RUnlock()
	return s.id
}

func New(httpClient *http.Client, url, username, password string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &Client{
		httpClient: httpClient,
		url:        url,
		username:   username,
		password:   password,
		token:      &sessionToken{m: new(sync.RWMutex)},
	}
}

type Client struct {
	httpClient *http.Client

	url      string
	username string
	password string

	token *sessionToken
}

// Login creates a session with the API. This operation exchanges user credentials supplied in the security context
// for a session identifier that is to be used for authenticating subsequent calls.
func (c *Client) Login() error {
	req := web.RequestConfig{
		URL:      fmt.Sprintf("%s%s", c.url, pathCISSession),
		Username: c.username,
		Password: c.password,
		Method:   http.MethodPost,
	}
	s := struct{ Value string }{}

	err := c.doOKWithDecode(req, &s)
	if err == nil {
		c.token.set(s.Value)
	}
	return err
}

// Logout terminates the validity of a session token.
func (c *Client) Logout() error {
	req := web.RequestConfig{
		URL:     fmt.Sprintf("%s%s", c.url, pathCISSession),
		Method:  http.MethodDelete,
		Headers: map[string]string{apiSessIDKey: c.token.get()},
	}

	resp, err := c.doOK(req)
	web.CloseBody(resp)
	c.token.set("")
	return err
}

// Ping sent a request to VCSA server to ensure the link is operating.
// In case of 401 error Ping tries to re authenticate.
func (c *Client) Ping() error {
	req := web.RequestConfig{
		URL:     fmt.Sprintf("%s%s?~action=get", c.url, pathCISSession),
		Method:  http.MethodPost,
		Headers: map[string]string{apiSessIDKey: c.token.get()},
	}
	resp, err := c.doOK(req)
	defer web.CloseBody(resp)
	if resp != nil && resp.StatusCode == http.StatusUnauthorized {
		return c.Login()
	}
	return err
}

func (c *Client) health(urlPath string) (string, error) {
	req := web.RequestConfig{
		URL:     fmt.Sprintf("%s%s", c.url, urlPath),
		Headers: map[string]string{apiSessIDKey: c.token.get()},
	}
	s := struct{ Value string }{}
	err := c.doOKWithDecode(req, &s)
	return s.Value, err
}

// ApplMgmt provides health status of applmgmt services.
func (c *Client) ApplMgmt() (string, error) {
	return c.health(pathHealthApplMgmt)
}

// DatabaseStorage provides health status of database storage health.
func (c *Client) DatabaseStorage() (string, error) {
	return c.health(pathHealthDatabaseStorage)
}

// Load provides health status of load health.
func (c *Client) Load() (string, error) {
	return c.health(pathHealthLoad)
}

// Mem provides health status of memory health.
func (c *Client) Mem() (string, error) {
	return c.health(pathHealthMem)
}

// SoftwarePackages provides information on available software updates available in remote VUM repository.
// Red indicates that security updates are available.
// Orange indicates that non-security updates are available.
// Green indicates that there are no updates available.
// Gray indicates that there was an error retrieving information on software updates.
func (c *Client) SoftwarePackages() (string, error) {
	return c.health(pathHealthSoftwarePackager)
}

// Storage provides health status of storage health.
func (c *Client) Storage() (string, error) {
	return c.health(pathHealthStorage)
}

// Swap provides health status of swap health.
func (c *Client) Swap() (string, error) {
	return c.health(pathHealthSwap)
}

// System provides overall health of system.
func (c *Client) System() (string, error) {
	return c.health(pathHealthSystem)
}

func (c *Client) do(req web.RequestConfig) (*http.Response, error) {
	httpReq, err := web.NewHTTPRequest(req)
	if err != nil {
		return nil, fmt.Errorf("error on creating http request to %s : %v", req.URL, err)
	}
	return c.httpClient.Do(httpReq)
}

func (c *Client) doOK(req web.RequestConfig) (*http.Response, error) {
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return resp, fmt.Errorf("%s returned %d", req.URL, resp.StatusCode)
	}
	return resp, nil
}

func (c *Client) doOKWithDecode(req web.RequestConfig, dst any) error {
	resp, err := c.doOK(req)
	defer web.CloseBody(resp)
	if err != nil {
		return err
	}

	err = json.NewDecoder(resp.Body).Decode(dst)
	if err != nil {
		return fmt.Errorf("error on decoding response from %s : %v", req.URL, err)
	}
	return nil
}
