// SPDX-License-Identifier: GPL-3.0-or-later

package dbdriver

import "strings"

// SanitizeDSN masks sensitive information in DSN for logging
func SanitizeDSN(dsn string) string {
	if dsn == "" {
		return "<empty>"
	}

	masked := dsn

	// Mask various password formats
	passwordKeys := []string{"PWD=", "pwd=", "Pwd=", "PASSWORD=", "password=", "Password="}
	for _, key := range passwordKeys {
		if idx := strings.Index(masked, key); idx != -1 {
			start := idx + len(key)
			end := strings.IndexAny(masked[start:], ";")
			if end == -1 {
				masked = masked[:start] + "***"
			} else {
				masked = masked[:start] + "***" + masked[start+end:]
			}
		}
	}

	// Mask authentication fields
	authKeys := []string{"AUTHENTICATION=", "Authentication=", "authentication="}
	for _, key := range authKeys {
		if idx := strings.Index(masked, key); idx != -1 {
			start := idx + len(key)
			end := strings.IndexAny(masked[start:], ";")
			if end == -1 {
				masked = masked[:start] + "***"
			} else {
				masked = masked[:start] + "***" + masked[start+end:]
			}
		}
	}

	return masked
}

// containsDB2Keywords checks if DSN contains DB2-specific keywords
func containsDB2Keywords(dsn string) bool {
	upperDSN := strings.ToUpper(dsn)
	db2Keywords := []string{
		"DATABASE=",
		"HOSTNAME=",
		"PROTOCOL=TCPIP",
		"UID=",
		"PWD=",
		"PORT=",
	}

	matchCount := 0
	for _, keyword := range db2Keywords {
		if strings.Contains(upperDSN, keyword) {
			matchCount++
		}
	}

	// If we have at least 3 DB2 keywords, it's likely a DB2 DSN
	return matchCount >= 3
}

// containsODBCKeywords checks if DSN contains ODBC-specific keywords
func containsODBCKeywords(dsn string) bool {
	upperDSN := strings.ToUpper(dsn)
	odbcKeywords := []string{
		"DRIVER=",
		"DSN=",
		"DRIVER={",
		"SYSTEM=", // AS/400 ODBC style
	}

	for _, keyword := range odbcKeywords {
		if strings.Contains(upperDSN, keyword) {
			return true
		}
	}

	return false
}
