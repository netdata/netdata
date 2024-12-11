// SPDX-License-Identifier: GPL-3.0-or-later

package openldap

import (
	"context"
	"github.com/go-ldap/ldap/v3"
)

func (c *Collector) collect() (map[string]int64, error) {
	if c.conn == nil {
		conn, err := c.establishConn()
		if err != nil {
			return nil, err
		}
		c.conn = conn
	}

	mx := make(map[string]int64)

	if err := c.collectMonitorCounters(mx); err != nil {
		c.Cleanup(context.Background())
		return nil, err
	}
	if err := c.collectOperations(mx); err != nil {
		c.Cleanup(context.Background())
		return nil, err
	}

	return mx, nil
}

func (c *Collector) doSearchRequest(req *ldap.SearchRequest, fn func(*ldap.Entry)) error {
	resp, err := c.conn.search(req)
	if err != nil {
		return err
	}

	for _, entry := range resp.Entries {
		if len(entry.Attributes) != 0 {
			fn(entry)
		}
	}

	return nil
}

func (c *Collector) establishConn() (ldapConn, error) {
	conn := c.newConn(c.Config)

	if err := conn.connect(); err != nil {
		return nil, err
	}

	return conn, nil
}
