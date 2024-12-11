// SPDX-License-Identifier: GPL-3.0-or-later

package openldap

import (
	"strconv"

	"github.com/go-ldap/ldap/v3"
)

const (
	attrMonitorCounter = "monitorCounter"
)

func (c *Collector) collectMonitorCounters(mx map[string]int64) error {
	req := newLdapMonitorCountersSearchRequest()

	dnMetricMap := map[string]string{
		"cn=Current,cn=Connections,cn=Monitor":  "current_connections",
		"cn=Total,cn=Connections,cn=Monitor":    "total_connections",
		"cn=Bytes,cn=Statistics,cn=Monitor":     "bytes_sent",
		"cn=Referrals,cn=Statistics,cn=Monitor": "referrals_sent",
		"cn=Entries,cn=Statistics,cn=Monitor":   "entries_sent",
		"cn=Write,cn=Waiters,cn=Monitor":        "write_waiters",
		"cn=Read,cn=Waiters,cn=Monitor":         "read_waiters",
	}

	return c.doSearchRequest(req, func(entry *ldap.Entry) {
		metric := dnMetricMap[entry.DN]
		if metric == "" {
			c.Debugf("skipping entry '%s'", entry.DN)
			return
		}

		s := entry.GetAttributeValue(attrMonitorCounter)
		if s == "" {
			c.Debugf("entry '%s' does not have attribute '%s'", entry.DN, attrMonitorCounter)
			return
		}

		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			c.Debugf("failed to parse entry '%s' value '%s': %v", entry.DN, s, err)
			return
		}

		mx[metric] = v
	})
}

func newLdapMonitorCountersSearchRequest() *ldap.SearchRequest {
	return ldap.NewSearchRequest(
		"cn=Monitor",
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		"(objectclass=monitorCounterObject)",
		[]string{attrMonitorCounter},
		nil,
	)
}
