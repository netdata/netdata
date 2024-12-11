// SPDX-License-Identifier: GPL-3.0-or-later

package zookeeper

import (
	"crypto/tls"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/tlscfg"
)

func (c *Collector) verifyConfig() error {
	if c.Address == "" {
		return errors.New("address not set")
	}
	return nil
}

func (c *Collector) initZookeeperFetcher() (fetcher, error) {
	var tlsConf *tls.Config
	var err error

	if c.UseTLS {
		tlsConf, err = tlscfg.NewTLSConfig(c.TLSConfig)
		if err != nil {
			return nil, fmt.Errorf("creating tls config : %v", err)
		}
	}

	sock := socket.New(socket.Config{
		Address:      c.Address,
		Timeout:      c.Timeout.Duration(),
		TLSConf:      tlsConf,
		MaxReadLines: 2000,
	})

	return &zookeeperFetcher{Client: sock}, nil
}
