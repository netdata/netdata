// SPDX-License-Identifier: GPL-3.0-or-later

package openldap

import (
	"github.com/go-ldap/ldap/v3"
)

func (l *OpenLDAP) collect() (map[string]int64, error) {
	if l.conn == nil {
		conn, err := l.establishConn()
		if err != nil {
			return nil, err
		}
		l.conn = conn
	}

	mx := make(map[string]int64)

	if err := l.collectMonitorCounters(mx); err != nil {
		l.Cleanup()
		return nil, err
	}
	if err := l.collectOperations(mx); err != nil {
		l.Cleanup()
		return nil, err
	}

	return mx, nil
}

func (l *OpenLDAP) doSearchRequest(req *ldap.SearchRequest, fn func(*ldap.Entry)) error {
	resp, err := l.conn.search(req)
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

func (l *OpenLDAP) establishConn() (ldapConn, error) {
	conn := l.newConn(l.Config)

	if err := conn.connect(); err != nil {
		return nil, err
	}

	return conn, nil
}
