// SPDX-License-Identifier: GPL-3.0-or-later

package phpfpm

import (
	"errors"
	"fmt"
	"os"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (p *Phpfpm) initClient() (client, error) {
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

func (p *Phpfpm) initHTTPClient() (*httpClient, error) {
	c, err := web.NewHTTPClient(p.ClientConfig)
	if err != nil {
		return nil, fmt.Errorf("create HTTPConfig client: %v", err)
	}

	p.Debugf("using HTTPConfig client: url='%s', timeout='%s'", p.URL, p.Timeout)

	return newHTTPClient(c, p.RequestConfig)
}

func (p *Phpfpm) initSocketClient() (*socketClient, error) {
	if _, err := os.Stat(p.Socket); err != nil {
		return nil, fmt.Errorf("the socket '%s' does not exist: %v", p.Socket, err)
	}

	p.Debugf("using socket client: socket='%s', timeout='%s', fcgi_path='%s'", p.Socket, p.Timeout, p.FcgiPath)

	return newSocketClient(p.Logger, p.Socket, p.Timeout.Duration(), p.FcgiPath), nil
}

func (p *Phpfpm) initTcpClient() (*tcpClient, error) {
	p.Debugf("using tcp client: address='%s', timeout='%s', fcgi_path='%s'", p.Address, p.Timeout, p.FcgiPath)

	return newTcpClient(p.Logger, p.Address, p.Timeout.Duration(), p.FcgiPath), nil
}
