// SPDX-License-Identifier: GPL-3.0-or-later

package openldap

import (
	"strconv"

	"github.com/go-ldap/ldap/v3"
)

const (
	attrMonitorOpInitiated = "monitorOpInitiated"
	attrMonitorOpCompleted = "monitorOpCompleted"
)

func (c *Collector) collectOperations(mx map[string]int64) error {
	req := newLdapOperationsSearchRequest()

	dnMetricMap := map[string]string{
		"cn=Bind,cn=Operations,cn=Monitor":    "bind_operations",
		"cn=Unbind,cn=Operations,cn=Monitor":  "unbind_operations",
		"cn=Add,cn=Operations,cn=Monitor":     "add_operations",
		"cn=Delete,cn=Operations,cn=Monitor":  "delete_operations",
		"cn=Modify,cn=Operations,cn=Monitor":  "modify_operations",
		"cn=Compare,cn=Operations,cn=Monitor": "compare_operations",
		"cn=Search,cn=Operations,cn=Monitor":  "search_operations",
	}

	return c.doSearchRequest(req, func(entry *ldap.Entry) {
		metric := dnMetricMap[entry.DN]
		if metric == "" {
			c.Debugf("skipping entry '%s'", entry.DN)
			return
		}

		attrs := map[string]string{
			"initiated": attrMonitorOpInitiated,
			"completed": attrMonitorOpCompleted,
		}

		for prefix, attr := range attrs {
			s := entry.GetAttributeValue(attr)
			if s == "" {
				c.Debugf("entry '%s' does not have attribute '%s'", entry.DN, attr)
				continue
			}
			v, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				c.Debugf("failed to parse entry '%s' value '%s': %v", entry.DN, s, err)
				continue
			}

			mx[prefix+"_"+metric] = v
			mx[prefix+"_operations"] += v
		}
	})
}

func newLdapOperationsSearchRequest() *ldap.SearchRequest {
	return ldap.NewSearchRequest(
		"cn=Operations,cn=Monitor",
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		"(objectclass=monitorOperation)",
		[]string{attrMonitorOpInitiated, attrMonitorOpCompleted},
		nil,
	)
}
