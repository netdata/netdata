// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/azureauth"
	"github.com/stretchr/testify/assert"
)

func TestCollector_Init(t *testing.T) {
	c := New()
	c.DSN = "sqlserver://localhost:1433"

	assert.NoError(t, c.Init(context.Background()))
}

func TestCollector_Init_EmptyDSN(t *testing.T) {
	c := New()
	c.DSN = ""

	assert.Error(t, c.Init(context.Background()))
}

func TestCollector_Init_InvalidAzureADConfig(t *testing.T) {
	c := New()
	c.AzureAD.Enabled = true
	c.AzureAD.Mode = "service_principal"
	c.AzureAD.ClientID = "client-id"
	c.AzureAD.TenantID = "tenant-id"
	// Missing client_secret.

	assert.Error(t, c.Init(context.Background()))
}

func TestCollector_openConnection_AzureADRequiresURLDSN(t *testing.T) {
	c := New()
	c.DSN = "server=localhost;database=master"
	c.AzureAD.Enabled = true
	c.AzureAD.Mode = azureauth.ModeDefault

	db, err := c.openConnection()
	assert.Nil(t, db)
	assert.ErrorContains(t, err, "error preparing Azure AD SQL Server DSN")
}

func TestCollector_Configuration(t *testing.T) {
	c := New()

	// Verify defaults
	assert.Equal(t, "sqlserver://localhost:1433", c.DSN)
}

func TestCollector_Charts(t *testing.T) {
	c := New()

	charts := c.Charts()
	assert.NotNil(t, charts)
	assert.NotEmpty(t, *charts)
}
