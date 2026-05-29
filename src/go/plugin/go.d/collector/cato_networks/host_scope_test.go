// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCatoSiteHostScope(t *testing.T) {
	tests := map[string]struct {
		accountID string
		site      *siteState
		wantEmpty bool
		wantHost  string
		wantLabel map[string]string
	}{
		"site name hostname with stable id scope": {
			accountID: "12345",
			site:      &siteState{ID: "1001", Name: "Paris Office", PopName: "POP-Paris"},
			wantHost:  "Paris Office",
			wantLabel: map[string]string{
				catoSiteScopeLabelKey: catoSiteScopeLabelValue,
				"cato_account_id":     "12345",
				"cato_site_id":        "1001",
				"cato_site_name":      "Paris Office",
				"cato_pop_name":       "POP-Paris",
			},
		},
		"empty site name falls back to site id hostname": {
			accountID: "12345",
			site:      &siteState{ID: "1001", PopName: "POP-Paris"},
			wantHost:  "cato-site-1001",
			wantLabel: map[string]string{
				catoSiteScopeLabelKey: catoSiteScopeLabelValue,
				"cato_account_id":     "12345",
				"cato_site_id":        "1001",
				"cato_site_name":      "",
				"cato_pop_name":       "POP-Paris",
			},
		},
		"nil site uses default scope": {
			accountID: "12345",
			wantEmpty: true,
		},
		"empty site id uses default scope": {
			accountID: "12345",
			site:      &siteState{Name: "Paris Office"},
			wantEmpty: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := catoSiteHostScope(tc.accountID, tc.site)
			if tc.wantEmpty {
				require.True(t, got.IsDefault())
				return
			}

			require.False(t, got.IsDefault())
			require.Equal(t, got.ScopeKey, got.GUID)
			require.Equal(t, tc.wantHost, got.Hostname)
			require.Equal(t, tc.wantLabel, got.Labels)
			require.Equal(t, got, catoSiteHostScope(tc.accountID, tc.site))
			require.NotEqual(t, got.ScopeKey, catoSiteHostScope("other-account", tc.site).ScopeKey)
			require.NotEqual(t, got.ScopeKey, catoSiteHostScope(tc.accountID, &siteState{ID: "1002"}).ScopeKey)
		})
	}
}
