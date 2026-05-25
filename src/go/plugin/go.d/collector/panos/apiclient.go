// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PaloAltoNetworks/pango"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

type panosAPIClient interface {
	op(cmd string) ([]byte, error)
	systemInfo() map[string]string
	closeIdleConnections()
}

type pangoOperator interface {
	Initialize() error
	Op(req any, vsys string, extras, ans any) ([]byte, error)
	RetrieveApiKey() error
	SystemInfo() map[string]string
}

type pangoAPIClient struct {
	client    pangoOperator
	transport *http.Transport

	vsys        string
	canRefresh  bool
	initialized bool
}

func newPangoAPIClient(cfg Config) (panosAPIClient, error) {
	apiURL, err := parseAPIURL(cfg.URL)
	if err != nil {
		return nil, err
	}

	transport, err := newPangoTransport(cfg.ClientConfig)
	if err != nil {
		return nil, err
	}

	fw := &pango.Firewall{
		Client: pango.Client{
			Hostname:          apiURL.hostname,
			Protocol:          apiURL.protocol,
			Port:              apiURL.port,
			Timeout:           timeoutSeconds(cfg.ClientConfig),
			Username:          cfg.Username,
			Password:          cfg.Password,
			ApiKey:            cfg.APIKey,
			Headers:           cfg.Headers,
			VerifyCertificate: !cfg.TLSConfig.InsecureSkipVerify,
			Transport:         transport,
			Logging:           pango.LogQuiet,
		},
	}

	return &pangoAPIClient{
		client:    &pangoFirewallOperator{fw: fw},
		transport: transport,
		vsys:      cfg.Vsys,
		canRefresh: cfg.Username != "" &&
			cfg.Password != "",
	}, nil
}

func (c *pangoAPIClient) op(cmd string) ([]byte, error) {
	if err := c.ensureInitialized(); err != nil {
		return nil, err
	}

	body, err := c.client.Op(cmd, c.vsys, nil, nil)
	if err == nil || !c.canRefresh || !isUnauthorizedError(err) {
		return body, sanitizePANOSAPIError(err)
	}

	if refreshErr := c.client.RetrieveApiKey(); refreshErr != nil {
		c.initialized = false
		return nil, fmt.Errorf("refresh PAN-OS API key after unauthorized response: %w", sanitizePANOSAPIError(refreshErr))
	}

	body, err = c.client.Op(cmd, c.vsys, nil, nil)
	return body, sanitizePANOSAPIError(err)
}

func (c *pangoAPIClient) ensureInitialized() error {
	if c.initialized {
		return nil
	}

	err := c.client.Initialize()
	if err == nil {
		c.initialized = true
		return nil
	}
	if !c.canRefresh || !isUnauthorizedError(err) {
		return sanitizePANOSAPIError(err)
	}

	if refreshErr := c.client.RetrieveApiKey(); refreshErr != nil {
		return fmt.Errorf("refresh PAN-OS API key after unauthorized initialization: %w", sanitizePANOSAPIError(refreshErr))
	}

	if err := c.client.Initialize(); err != nil {
		return fmt.Errorf("re-initialize PAN-OS API client after key refresh: %w", sanitizePANOSAPIError(err))
	}

	c.initialized = true
	return nil
}

func (c *pangoAPIClient) systemInfo() map[string]string {
	info := c.client.SystemInfo()
	if len(info) == 0 {
		return nil
	}
	cp := make(map[string]string, len(info))
	for k, v := range info {
		cp[k] = v
	}
	return cp
}

func (c *pangoAPIClient) closeIdleConnections() {
	if c.transport != nil {
		c.transport.CloseIdleConnections()
	}
}

type pangoFirewallOperator struct {
	fw *pango.Firewall
}

func (p *pangoFirewallOperator) Initialize() error {
	return p.fw.Initialize()
}

func (p *pangoFirewallOperator) Op(req any, vsys string, extras, ans any) ([]byte, error) {
	return p.fw.Op(req, vsys, extras, ans)
}

func (p *pangoFirewallOperator) RetrieveApiKey() error {
	return p.fw.RetrieveApiKey()
}

func (p *pangoFirewallOperator) SystemInfo() map[string]string {
	return p.fw.SystemInfo
}

func isUnauthorizedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "code 16") ||
		strings.Contains(msg, "code: 16") ||
		strings.Contains(msg, "code 22") ||
		strings.Contains(msg, "code: 22") ||
		strings.Contains(msg, "code 403") ||
		strings.Contains(msg, "code: 403") ||
		strings.Contains(msg, "forbidden") ||
		strings.Contains(msg, "session timed out")
}

type panosAPIURL struct {
	protocol string
	hostname string
	port     uint
}

func parseAPIURL(rawURL string) (panosAPIURL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return panosAPIURL{}, fmt.Errorf("parse url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return panosAPIURL{}, fmt.Errorf("config: url scheme must be http or https")
	}
	if u.User != nil {
		return panosAPIURL{}, errors.New("config: url must not include embedded credentials")
	}
	if u.Hostname() == "" {
		return panosAPIURL{}, errors.New("config: url hostname not configured")
	}
	if u.Path != "" && u.Path != "/" && u.Path != "/api" {
		return panosAPIURL{}, fmt.Errorf("config: url path must be empty, /, or /api")
	}

	var port uint
	if u.Port() != "" {
		v, err := strconv.ParseUint(u.Port(), 10, 16)
		if err != nil {
			return panosAPIURL{}, fmt.Errorf("parse url port: %w", err)
		}
		port = uint(v)
	}

	hostname := u.Hostname()
	if strings.Contains(hostname, ":") {
		hostname = "[" + hostname + "]"
	}

	return panosAPIURL{
		protocol: u.Scheme,
		hostname: hostname,
		port:     port,
	}, nil
}

func newPangoTransport(cfg web.ClientConfig) (*http.Transport, error) {
	client, err := web.NewHTTPClient(cfg)
	if err != nil {
		return nil, err
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		return nil, errors.New("PAN-OS SDK requires an HTTP/1.x transport")
	}
	transport.MaxConnsPerHost = 2
	transport.MaxIdleConnsPerHost = 2
	return transport, nil
}

func timeoutSeconds(cfg web.ClientConfig) int {
	d := cfg.Timeout.Duration()
	if d <= 0 {
		return 10
	}
	return max(1, int(math.Ceil(d.Seconds())))
}

var secretParamRE = regexp.MustCompile(`(?i)\b((?:key|apikey|api_key|password|pass)=)[^&\s]+`)

func sanitizePANOSAPIError(err error) error {
	if err == nil {
		return nil
	}
	msg := secretParamRE.ReplaceAllString(err.Error(), "${1}<redacted>")
	return errors.New(msg)
}
