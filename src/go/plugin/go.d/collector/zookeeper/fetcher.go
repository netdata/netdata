// SPDX-License-Identifier: GPL-3.0-or-later

package zookeeper

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"
)

type fetcher interface {
	fetch(command string) ([]string, error)
}

type zookeeperFetcher struct {
	socket.Client
}

func (c *zookeeperFetcher) fetch(command string) (rows []string, err error) {
	if err = c.Connect(); err != nil {
		return nil, err
	}
	defer func() { _ = c.Disconnect() }()

	if err := c.Command(command, func(b []byte) (bool, error) {
		rows = append(rows, string(b))
		return true, nil
	}); err != nil {
		return nil, err
	}

	return rows, nil
}
