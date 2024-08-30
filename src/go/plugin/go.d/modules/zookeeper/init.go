// SPDX-License-Identifier: GPL-3.0-or-later

package zookeeper

import (
	"crypto/tls"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/tlscfg"
)

func (z *Zookeeper) verifyConfig() error {
	if z.Address == "" {
		return errors.New("address not set")
	}
	return nil
}

func (z *Zookeeper) initZookeeperFetcher() (fetcher, error) {
	var tlsConf *tls.Config
	var err error

	if z.UseTLS {
		tlsConf, err = tlscfg.NewTLSConfig(z.TLSConfig)
		if err != nil {
			return nil, fmt.Errorf("creating tls config : %v", err)
		}
	}

	sock := socket.New(socket.Config{
		Address:        z.Address,
		ConnectTimeout: z.Timeout.Duration(),
		ReadTimeout:    z.Timeout.Duration(),
		WriteTimeout:   z.Timeout.Duration(),
		TLSConf:        tlsConf,
	})

	return &zookeeperFetcher{Client: sock}, nil
}
