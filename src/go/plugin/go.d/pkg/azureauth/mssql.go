// SPDX-License-Identifier: GPL-3.0-or-later

package azureauth

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	MSSQLDriverName      = "sqlserver"
	MSSQLAzureDriverName = "azuresql"

	mssqlFedAuthDefault          = "ActiveDirectoryDefault"
	mssqlFedAuthManagedIdentity  = "ActiveDirectoryManagedIdentity"
	mssqlFedAuthServicePrincipal = "ActiveDirectoryServicePrincipal"
)

func MSSQLDriver(cfg Config) string {
	if cfg.Enabled {
		return MSSQLAzureDriverName
	}
	return MSSQLDriverName
}

func BuildMSSQLAzureADDSN(baseDSN string, cfg Config) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", err
	}
	if !cfg.Enabled {
		return baseDSN, nil
	}

	u, err := url.Parse(baseDSN)
	if err != nil {
		return "", fmt.Errorf("parsing SQL Server DSN: %w", err)
	}
	if !strings.EqualFold(u.Scheme, "sqlserver") {
		return "", fmt.Errorf("azure_ad requires URL DSN with sqlserver scheme, got %q", u.Scheme)
	}

	q := u.Query()
	q.Del("fedauth")
	q.Del("user id")

	switch cfg.normalizedMode() {
	case ModeServicePrincipal:
		q.Set("fedauth", mssqlFedAuthServicePrincipal)
		userID := cfg.ClientID
		if tenantID := strings.TrimSpace(cfg.TenantID); tenantID != "" {
			userID = userID + "@" + tenantID
		}
		u.User = url.UserPassword(userID, cfg.ClientSecret)
	case ModeManagedIdentity:
		q.Set("fedauth", mssqlFedAuthManagedIdentity)
		u.User = nil
		if id := strings.TrimSpace(cfg.ManagedIdentityClientID); id != "" {
			q.Set("user id", id)
		}
	case ModeDefault:
		q.Set("fedauth", mssqlFedAuthDefault)
		u.User = nil
	default:
		return "", fmt.Errorf("unsupported azure_ad.mode %q", cfg.Mode)
	}

	u.RawQuery = q.Encode()

	return u.String(), nil
}
