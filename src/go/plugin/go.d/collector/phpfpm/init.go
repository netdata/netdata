// SPDX-License-Identifier: GPL-3.0-or-later

package phpfpm

import (
	"errors"
	"fmt"
	"os"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (c *Collector) initClient() (client, error) {
	if c.Socket != "" {
		return c.initSocketClient()
	}
	if c.Address != "" {
		return c.initTcpClient()
	}
	if c.URL != "" {
		return c.initHTTPClient()
	}

	return nil, errors.New("neither 'socket' nor 'url' set")
}

func (c *Collector) initHTTPClient() (*httpClient, error) {
	cli, err := web.NewHTTPClient(c.ClientConfig)
	if err != nil {
		return nil, fmt.Errorf("create HTTP client: %v", err)
	}

	c.Debugf("using HTTP client: url='%s', timeout='%s'", c.URL, c.Timeout)

	return newHTTPClient(cli, c.RequestConfig)
}

func (c *Collector) initSocketClient() (*socketClient, error) {
	if _, err := os.Stat(c.Socket); err != nil {
		return nil, fmt.Errorf("the socket '%s' does not exist: %v", c.Socket, err)
	}

	c.Debugf("using socket client: socket='%s', timeout='%s', fcgi_path='%s'", c.Socket, c.Timeout, c.FcgiPath)

	return newSocketClient(c.Logger, c.Socket, c.Timeout.Duration(), c.FcgiPath), nil
}

func (c *Collector) initTcpClient() (*tcpClient, error) {
	c.Debugf("using tcp client: address='%s', timeout='%s', fcgi_path='%s'", c.Address, c.Timeout, c.FcgiPath)

	return newTcpClient(c.Logger, c.Address, c.Timeout.Duration(), c.FcgiPath), nil
}
