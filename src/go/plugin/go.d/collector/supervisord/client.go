// SPDX-License-Identifier: GPL-3.0-or-later

package supervisord

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/mattn/go-xmlrpc"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

type supervisorClient interface {
	getAllProcessInfo() ([]processStatus, error)
	closeIdleConnections()
}

type supervisorRPCClient struct {
	client *xmlrpc.Client
}

func newSupervisorRPCClient(ctx context.Context, serverURL *url.URL, cfg web.ClientConfig) (supervisorClient, error) {
	var httpClient *http.Client
	var err error
	rpcURL := serverURL.String()
	switch serverURL.Scheme {
	case "http", "https":
		httpClient, err = web.NewHTTPClient(ctx, cfg)
	case "unix":
		if cfg.ForceHTTP2 {
			return nil, errors.New("unexpected HTTPConfig client transport")
		}
		t, err := web.NewHTTPTransport(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("create HTTP transport: %w", err)
		}
		t.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: cfg.Timeout.Duration()}
			return d.DialContext(ctx, "unix", serverURL.Path)
		}
		httpClient = web.NewHTTPClientWithTransport(cfg, t)
		rpcURL = "http://unix/RPC2"
	default:
		return nil, fmt.Errorf("unexpected URL scheme: %s", serverURL)
	}
	if err != nil {
		return nil, fmt.Errorf("create HTTP client: %w", err)
	}
	c := xmlrpc.NewClient(rpcURL)
	c.HttpClient = httpClient
	return &supervisorRPCClient{client: c}, nil
}

// http://supervisord.org/api.html#process-control
type processStatus struct {
	name       string // name of the process.
	group      string // name of the process’ group.
	start      int    // UNIX timestamp of when the process was started.
	stop       int    // UNIX timestamp of when the process last ended, or 0 if the process has never been stopped.
	now        int    // UNIX timestamp of the current time, which can be used to calculate process up-time.
	state      int    // state code.
	stateName  string // string description of state.
	exitStatus int    // exit status (errorlevel) of process, or 0 if the process is still running.
}

func (c *supervisorRPCClient) getAllProcessInfo() ([]processStatus, error) {
	const fn = "supervisor.getAllProcessInfo"
	resp, err := c.client.Call(fn)
	if err != nil {
		return nil, fmt.Errorf("error on '%s' function call: %v", fn, err)
	}
	return parseGetAllProcessInfo(resp)
}

func (c *supervisorRPCClient) closeIdleConnections() {
	c.client.HttpClient.CloseIdleConnections()
}

func parseGetAllProcessInfo(resp any) ([]processStatus, error) {
	arr, ok := resp.(xmlrpc.Array)
	if !ok {
		return nil, fmt.Errorf("unexpected response type, want=xmlrpc.Array, got=%T", resp)
	}

	var info []processStatus

	for _, item := range arr {
		s, ok := item.(xmlrpc.Struct)
		if !ok {
			continue
		}

		var p processStatus
		for k, v := range s {
			switch strings.ToLower(k) {
			case "name":
				p.name, _ = v.(string)
			case "group":
				p.group, _ = v.(string)
			case "start":
				p.start, _ = v.(int)
			case "stop":
				p.stop, _ = v.(int)
			case "now":
				p.now, _ = v.(int)
			case "state":
				p.state, _ = v.(int)
			case "statename":
				p.stateName, _ = v.(string)
			case "exitstatus":
				p.exitStatus, _ = v.(int)
			}
		}
		if p.name != "" && p.group != "" && p.stateName != "" {
			info = append(info, p)
		}
	}
	return info, nil
}
