// SPDX-License-Identifier: GPL-3.0-or-later

package cloudauth

import "strings"

const (
	ProviderNone    = "none"
	ProviderAzureAD = "azure_ad"
)

func normalizeProviderValue(provider string) string {
	p := strings.ToLower(strings.TrimSpace(provider))
	if p == "" {
		return ProviderNone
	}
	return p
}
