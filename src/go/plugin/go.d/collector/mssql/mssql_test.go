// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cloudauth"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cloudauth/sqladapter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	c.CloudAuth.Provider = cloudauth.ProviderAzureAD
	c.CloudAuth.AzureAD = &cloudauth.AzureADAuthConfig{
		Mode:     cloudauth.AzureADAuthModeServicePrincipal,
		ClientID: "client-id",
		TenantID: "tenant-id",
	}
	// Missing client_secret.

	assert.Error(t, c.Init(context.Background()))
}

func TestCollector_openConnection_AzureADRequiresURLDSN(t *testing.T) {
	c := New()
	c.DSN = "server=localhost;database=master"
	c.CloudAuth.Provider = cloudauth.ProviderAzureAD
	c.CloudAuth.AzureAD = &cloudauth.AzureADAuthConfig{Mode: cloudauth.AzureADAuthModeDefault}

	db, err := c.openConnection()
	assert.Nil(t, db)
	assert.ErrorContains(t, err, "error preparing cloud auth SQL Server DSN")
}

func TestCollector_resolveConnectionParams_AzureADRewritesDSNAndDriver(t *testing.T) {
	c := New()
	c.DSN = "sqlserver://localhost:1433?database=master"
	c.CloudAuth.Provider = cloudauth.ProviderAzureAD
	c.CloudAuth.AzureAD = &cloudauth.AzureADAuthConfig{Mode: cloudauth.AzureADAuthModeDefault}

	driverName, dsn, err := c.resolveConnectionParams()
	require.NoError(t, err)

	assert.Equal(t, sqladapter.MSSQLAzureDriverName, driverName)
	assert.Contains(t, dsn, "fedauth=ActiveDirectoryDefault")
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

func TestParseMajorVersion(t *testing.T) {
	tests := []struct {
		version  string
		expected int
	}{
		{"16.0.4175.1", 16},
		{"15.0.4198.2", 15},
		{"14.0.3456.2", 14},
		{"13.0.6300.2", 13},
		{"12.0.6024.0", 12},
		{"11.0.7001.0", 11},
		{"", 0},
		{"invalid", 0},
		{"abc.def", 0},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			assert.Equal(t, tt.expected, parseMajorVersion(tt.version))
		})
	}
}

func TestCleanAGName(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"MyAG", "myag"},
		{"My AG-1", "my_ag_1"},
		{"SERVER\\INSTANCE", "server_instance"},
		{"ag.with.dots", "ag_with_dots"},
		{"simple", "simple"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, cleanAGName(tt.name))
		})
	}
}

func TestCleanAGReplicaName(t *testing.T) {
	assert.Equal(t, "myag_server1", cleanAGReplicaName("MyAG", "SERVER1"))
	assert.Equal(t, "prod_ag_node1_instance", cleanAGReplicaName("Prod-AG", "Node1.Instance"))
}

func TestCleanAGDatabaseReplicaName(t *testing.T) {
	assert.Equal(t, "myag_server1_testdb", cleanAGDatabaseReplicaName("MyAG", "SERVER1", "TestDB"))
}

func TestAGDatabaseReplicaQuery(t *testing.T) {
	assert.Equal(t, queryAGDatabaseReplicas12, agDatabaseReplicaQuery(11))
	assert.Equal(t, queryAGDatabaseReplicas14, agDatabaseReplicaQuery(12))
	assert.Equal(t, queryAGDatabaseReplicas16, agDatabaseReplicaQuery(13))
	assert.Equal(t, queryAGDatabaseReplicas16, agDatabaseReplicaQuery(14))
	assert.Equal(t, queryAGDatabaseReplicas16, agDatabaseReplicaQuery(15))
	assert.Equal(t, queryAGDatabaseReplicas16, agDatabaseReplicaQuery(16))
}
