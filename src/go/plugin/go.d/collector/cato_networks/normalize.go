// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"strings"

	catomodels "github.com/catonetworks/cato-go-sdk/models"
	catoscalars "github.com/catonetworks/cato-go-sdk/scalars"
)

func derefZero[T any](v *T) T {
	if v == nil {
		var zero T
		return zero
	}
	return *v
}

func normalizeName(v string) string {
	return strings.TrimSpace(v)
}

func normalizeStatus(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "unknown"
	}
	return strings.ToLower(strings.ReplaceAll(v, " ", "_"))
}

func connectivityStatusString(v *catomodels.ConnectivityStatus) string {
	if v == nil {
		return ""
	}
	return string(*v)
}

func operationalStatusString(v *catoscalars.OperationalStatus) string {
	if v == nil {
		return ""
	}
	return v.GetString()
}

func siteDisplayName(siteID string, siteNames map[string]string, infoName, fallbackName string) string {
	switch {
	case normalizeName(infoName) != "":
		return normalizeName(infoName)
	case normalizeName(fallbackName) != "":
		return normalizeName(fallbackName)
	case normalizeName(siteNames[siteID]) != "":
		return normalizeName(siteNames[siteID])
	default:
		return siteID
	}
}

func interfaceKey(id, name string) string {
	if strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id)
	}
	if strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return "all"
}
