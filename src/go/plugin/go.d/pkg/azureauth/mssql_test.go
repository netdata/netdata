// SPDX-License-Identifier: GPL-3.0-or-later

package azureauth

import (
	"net/url"
	"testing"
)

func TestMSSQLDriver(t *testing.T) {
	if got := MSSQLDriver(Config{}); got != MSSQLDriverName {
		t.Fatalf("unexpected driver: %q", got)
	}
	if got := MSSQLDriver(Config{Enabled: true}); got != MSSQLAzureDriverName {
		t.Fatalf("unexpected azure driver: %q", got)
	}
}

func TestBuildMSSQLAzureADDSN(t *testing.T) {
	base := "sqlserver://localhost:1433?database=master"

	t.Run("service principal", func(t *testing.T) {
		cfg := Config{
			Enabled:      true,
			Mode:         ModeServicePrincipal,
			TenantID:     "tenant",
			ClientID:     "client",
			ClientSecret: "secret",
		}

		dsn, err := BuildMSSQLAzureADDSN(base, cfg)
		if err != nil {
			t.Fatalf("BuildMSSQLAzureADDSN() unexpected error: %v", err)
		}

		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatalf("url.Parse() unexpected error: %v", err)
		}

		if got := u.Query().Get("fedauth"); got != "ActiveDirectoryServicePrincipal" {
			t.Fatalf("unexpected fedauth: %q", got)
		}
		if got := u.User.Username(); got != "client@tenant" {
			t.Fatalf("unexpected username: %q", got)
		}
		if pass, ok := u.User.Password(); !ok || pass != "secret" {
			t.Fatalf("unexpected password")
		}
	})

	t.Run("managed identity with client id", func(t *testing.T) {
		cfg := Config{
			Enabled:                 true,
			Mode:                    ModeManagedIdentity,
			ManagedIdentityClientID: "mi-client-id",
		}

		dsn, err := BuildMSSQLAzureADDSN(base, cfg)
		if err != nil {
			t.Fatalf("BuildMSSQLAzureADDSN() unexpected error: %v", err)
		}

		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatalf("url.Parse() unexpected error: %v", err)
		}

		if got := u.Query().Get("fedauth"); got != "ActiveDirectoryManagedIdentity" {
			t.Fatalf("unexpected fedauth: %q", got)
		}
		if got := u.Query().Get("user id"); got != "mi-client-id" {
			t.Fatalf("unexpected user id: %q", got)
		}
		if u.User != nil {
			t.Fatalf("expected no user info for managed identity")
		}
	})

	t.Run("default credential", func(t *testing.T) {
		cfg := Config{
			Enabled: true,
			Mode:    ModeDefault,
		}

		dsn, err := BuildMSSQLAzureADDSN(base, cfg)
		if err != nil {
			t.Fatalf("BuildMSSQLAzureADDSN() unexpected error: %v", err)
		}

		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatalf("url.Parse() unexpected error: %v", err)
		}

		if got := u.Query().Get("fedauth"); got != "ActiveDirectoryDefault" {
			t.Fatalf("unexpected fedauth: %q", got)
		}
		if u.User != nil {
			t.Fatalf("expected no user info for default credential")
		}
	})

	t.Run("invalid scheme", func(t *testing.T) {
		cfg := Config{Enabled: true, Mode: ModeDefault}
		_, err := BuildMSSQLAzureADDSN("server=localhost;database=master", cfg)
		if err == nil {
			t.Fatalf("expected error for non-URL DSN")
		}
	})
}
