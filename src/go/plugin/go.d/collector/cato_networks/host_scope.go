// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"strings"

	"github.com/google/uuid"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
)

const (
	catoSiteScopeLabelKey   = "_vnode_type"
	catoSiteScopeLabelValue = "cato_site"
	catoSiteGUIDPrefix      = "cato_networks:"
)

func (c *Collector) siteHostScope(site *siteState) metrix.HostScope {
	return catoSiteHostScope(c.AccountID, site)
}

func catoSiteHostScope(accountID string, site *siteState) metrix.HostScope {
	if site == nil {
		return metrix.HostScope{}
	}
	siteID := strings.TrimSpace(site.ID)
	if siteID == "" {
		return metrix.HostScope{}
	}

	guid := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(catoSiteGUIDPrefix+strings.TrimSpace(accountID)+":site:"+siteID)).String()
	scope := metrix.HostScope{
		ScopeKey: guid,
		GUID:     guid,
		Hostname: catoSiteHostname(siteID, site.Name),
		Labels: map[string]string{
			catoSiteScopeLabelKey: catoSiteScopeLabelValue,
			"cato_account_id":     strings.TrimSpace(accountID),
			"cato_site_id":        siteID,
			"cato_site_name":      site.Name,
			"cato_pop_name":       site.PopName,
		},
	}
	if hostScopeIsSafe(scope) {
		return scope
	}

	scope.Hostname = "cato-site-" + siteID
	if hostScopeIsSafe(scope) {
		return scope
	}

	scope.Hostname = "cato-site-" + guid
	if hostScopeIsSafe(scope) {
		return scope
	}
	return metrix.HostScope{}
}

func catoSiteHostname(siteID, siteName string) string {
	if name := strings.TrimSpace(siteName); name != "" {
		return name
	}
	return "cato-site-" + siteID
}

func hostScopeIsSafe(scope metrix.HostScope) bool {
	_, err := chartemit.PrepareHostInfo(netdataapi.HostInfo{
		GUID:     scope.GUID,
		Hostname: scope.Hostname,
		Labels:   scope.Labels,
	})
	return err == nil
}
