// SPDX-License-Identifier: GPL-3.0-or-later

package openldap

import (
	"fmt"
	"log"
	"strconv"

	"github.com/go-ldap/ldap/v3"
)

func (l *OpenLDAP) collect() (map[string]int64, error) {

	ldapURL := l.LDAP_URL          // Change this to your LDAP server URL
	adminDN := l.DistinguishedName // Change this to the correct admin DN
	password := l.Password         // Change this to the admin password

	mx := make(map[string]int64)

	// Connect to the LDAP server
	conn, err := ldap.DialURL(ldapURL)
	if err != nil {
		log.Fatalf("Failed to connect to LDAP: %v", err)
	}
	defer conn.Close()

	// Bind with admin credentials
	err = conn.Bind(adminDN, password)
	if err != nil {
		log.Fatalf("LDAP bind failed: %v", err)
	}

	// LDAP base DNs and metrics to fetch
	metrics := []struct {
		// Description string
		BaseDN        string
		Attribute     string
		InternalDimID string
	}{
		{"cn=Total,cn=Connections,cn=Monitor", "monitorCounter", "total_connections"},
		{"cn=Bytes,cn=Statistics,cn=Monitor", "monitorCounter", "bytes_sent"},
		{"cn=Operations,cn=Monitor", "monitorOpCompleted", "completed_operations"},
		{"cn=Operations,cn=Monitor", "monitorOpInitiated", "initiated_operations"},
		{"cn=Referrals,cn=Statistics,cn=Monitor", "monitorCounter", "referrals_sent"},
		{"cn=Entries,cn=Statistics,cn=Monitor", "monitorCounter", "entries_sent"},
		{"cn=Bind,cn=Operations,cn=Monitor", "monitorOpCompleted", "bind_operations"},
		{"cn=Unbind,cn=Operations,cn=Monitor", "monitorOpCompleted", "unbind_operations"},
		{"cn=Add,cn=Operations,cn=Monitor", "monitorOpInitiated", "add_operations"},
		{"cn=Delete,cn=Operations,cn=Monitor", "monitorOpCompleted", "delete_operations"},
		{"cn=Modify,cn=Operations,cn=Monitor", "monitorOpCompleted", "modify_operations"},
		{"cn=Compare,cn=Operations,cn=Monitor", "monitorOpCompleted", "compare_operations"},
		{"cn=Search,cn=Operations,cn=Monitor", "monitorOpCompleted", "search_operations"},
		{"cn=Write,cn=Waiters,cn=Monitor", "monitorCounter", "write_waiters"},
		{"cn=Read,cn=Waiters,cn=Monitor", "monitorCounter", "read_waiters"},
	}

	// Fetch each metric and display the result
	for _, metric := range metrics {
		err = fetchMetric(mx, conn, metric.InternalDimID, metric.BaseDN, metric.Attribute)
		if err != nil {
			log.Printf("Failed to fetch %s: %v\n", metric.InternalDimID, err)
		}
	}

	return mx, nil
}

// Function to perform the search and display the metric
func fetchMetric(mx map[string]int64, conn *ldap.Conn, internalDimID string, baseDN, attribute string) error {
	// Perform an LDAP search request
	searchRequest := ldap.NewSearchRequest(
		baseDN,               // Base DN to search
		ldap.ScopeBaseObject, // Search only the base DN
		ldap.NeverDerefAliases,
		0, 0, false,
		"(objectclass=*)",   // Search all objects
		[]string{attribute}, // Attributes to fetch
		nil,
	)

	searchResult, err := conn.Search(searchRequest)
	if err != nil {
		return fmt.Errorf("search failed: %v", err)
	}

	// Print the metric result
	for _, entry := range searchResult.Entries {
		fmt.Printf("%s: %s\n", internalDimID, entry.GetAttributeValue(attribute))
		mx[internalDimID] = *parseInt(entry.GetAttributeValue(attribute))

	}
	return nil
}

func parseInt(value string) *int64 {
	v, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil
	}
	return &v
}
