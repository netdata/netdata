// SPDX-License-Identifier: GPL-3.0-or-later

package sqladapter

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cloudauth"
)

const (
	MSSQLDriverName      = "sqlserver"
	MSSQLAzureDriverName = "azuresql"

	mssqlFedAuthDefault          = "ActiveDirectoryDefault"
	mssqlFedAuthManagedIdentity  = "ActiveDirectoryManagedIdentity"
	mssqlFedAuthServicePrincipal = "ActiveDirectoryServicePrincipal"
)

func MSSQLDriver(cfg cloudauth.Config) string {
	if cfg.IsProvider(cloudauth.ProviderAzureAD) {
		return MSSQLAzureDriverName
	}
	return MSSQLDriverName
}

func BuildMSSQLAzureADDSN(baseDSN string, cfg cloudauth.Config) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", err
	}
	if !cfg.IsProvider(cloudauth.ProviderAzureAD) {
		return baseDSN, nil
	}

	aadCfg := cloudauth.AzureADAuthConfig{}
	if cfg.AzureAD != nil {
		aadCfg = *cfg.AzureAD
	}

	u, err := url.Parse(baseDSN)
	if err != nil {
		return "", fmt.Errorf("parsing SQL Server DSN: %w", err)
	}
	if !strings.EqualFold(u.Scheme, "sqlserver") {
		return "", fmt.Errorf("cloud_auth.provider %q requires URL DSN with sqlserver scheme, got %q", cloudauth.ProviderAzureAD, u.Scheme)
	}

	q := u.Query()
	for key := range q {
		switch strings.ToLower(key) {
		case "fedauth", "user id", "password":
			q.Del(key)
		}
	}

	switch aadCfg.NormalizedMode() {
	case cloudauth.AzureADAuthModeServicePrincipal:
		sp := aadCfg.ModeServicePrincipal
		if sp == nil {
			return "", fmt.Errorf("unsupported cloud_auth.azure_ad.mode %q", aadCfg.Mode)
		}
		q.Set("fedauth", mssqlFedAuthServicePrincipal)
		clientID := strings.TrimSpace(sp.ClientID)
		clientSecret := strings.TrimSpace(sp.ClientSecret)
		userID := clientID
		if tenantID := strings.TrimSpace(sp.TenantID); tenantID != "" {
			userID = userID + "@" + tenantID
		}
		u.User = url.UserPassword(userID, clientSecret)
	case cloudauth.AzureADAuthModeManagedIdentity:
		mi := aadCfg.ModeManagedIdentity
		q.Set("fedauth", mssqlFedAuthManagedIdentity)
		u.User = nil
		if mi != nil && strings.TrimSpace(mi.ClientID) != "" {
			id := strings.TrimSpace(mi.ClientID)
			q.Set("user id", id)
		}
	case cloudauth.AzureADAuthModeDefault:
		q.Set("fedauth", mssqlFedAuthDefault)
		u.User = nil
	default:
		return "", fmt.Errorf("unsupported cloud_auth.azure_ad.mode %q", aadCfg.Mode)
	}

	u.RawQuery = q.Encode()

	return u.String(), nil
}
