// SPDX-License-Identifier: GPL-3.0-or-later

package openldap

import (
	"net"

	"github.com/go-ldap/ldap/v3"

	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
)

type ldapConn interface {
	connect() error
	disconnect() error
	search(*ldap.SearchRequest) (*ldap.SearchResult, error)
}

func newLdapConn(cfg Config) ldapConn {
	return &ldapClient{Config: cfg}
}

type ldapClient struct {
	Config

	conn *ldap.Conn
}

func (c *ldapClient) search(req *ldap.SearchRequest) (*ldap.SearchResult, error) {
	return c.conn.Search(req)
}

func (c *ldapClient) connect() error {
	opts, err := c.connectOpts()
	if err != nil {
		return err
	}

	conn, err := ldap.DialURL(c.URL, opts...)
	if err != nil {
		return err
	}

	if c.Password == "" {
		err = conn.UnauthenticatedBind(c.Username)
	} else {
		err = conn.Bind(c.Username, c.Password)
	}
	if err != nil {
		_ = conn.Close()
		return err
	}

	c.conn = conn

	return nil
}

func (c *ldapClient) connectOpts() ([]ldap.DialOpt, error) {
	d := &net.Dialer{
		Timeout: c.Timeout.Duration(),
	}

	opts := []ldap.DialOpt{ldap.DialWithDialer(d)}

	tlsConf, err := tlscfg.NewTLSConfig(c.TLSConfig)
	if err != nil {
		return nil, err
	}
	if tlsConf != nil {
		opts = append(opts, ldap.DialWithTLSConfig(tlsConf))
	}

	return opts, nil
}

func (c *ldapClient) disconnect() error {
	defer func() { c.conn = nil }()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
