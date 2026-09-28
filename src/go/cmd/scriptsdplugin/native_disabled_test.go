// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !scripts_native_dev

package main

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/require"
)

func TestNativeAbsentFromProductionRegistry(t *testing.T) {
	_, nagios := collectorapi.DefaultRegistry.Lookup("nagios")
	_, native := collectorapi.DefaultRegistry.Lookup("native")
	require.True(t, nagios)
	require.False(t, native, "native must not be exposed by a production build")
}
