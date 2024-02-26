// SPDX-License-Identifier: GPL-3.0-or-later

package phpfpm

import (
	"errors"
	"fmt"
	"os"

	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

func (p Phpfpm) initClient() (client, error) {
	if p.Socket != "" {
		return p.initSocketClient()
	}
	if p.Address != "" {
		return p.initTcpClient()
	}
	if p.URL != "" {
		return p.initHTTPClient()
	}
	return nil, errors.New("neither 'socket' nor 'url' set")
}

func (p Phpfpm) initHTTPClient() (*httpClient, error) {
	c, err := web.NewHTTPClient(p.Client)
	if err != nil {
		return nil, fmt.Errorf("create HTTP client: %v", err)
	}
	p.Debugf("using HTTP client, URL: %s", p.URL)
	p.Debugf("using timeout: %s", p.Timeout.Duration)
	return newHTTPClient(c, p.Request)
}

func (p Phpfpm) initSocketClient() (*socketClient, error) {
	if _, err := os.Stat(p.Socket); err != nil {
		return nil, fmt.Errorf("the socket '%s' does not exist: %v", p.Socket, err)
	}
	p.Debugf("using socket client: %s", p.Socket)
	p.Debugf("using timeout: %s", p.Timeout.Duration)
	p.Debugf("using fcgi path: %s", p.FcgiPath)
	return newSocketClient(p.Socket, p.Timeout.Duration, p.FcgiPath), nil
}

func (p Phpfpm) initTcpClient() (*tcpClient, error) {
	p.Debugf("using tcp client: %s", p.Address)
	p.Debugf("using timeout: %s", p.Timeout.Duration)
	p.Debugf("using fcgi path: %s", p.FcgiPath)
	return newTcpClient(p.Address, p.Timeout.Duration, p.FcgiPath), nil
}
