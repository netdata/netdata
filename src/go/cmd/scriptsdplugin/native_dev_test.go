// SPDX-License-Identifier: GPL-3.0-or-later

//go:build scripts_native_dev

package main

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/require"
)

func TestNativeAvailableInDevelopmentRegistry(t *testing.T) {
	_, nagios := collectorapi.DefaultRegistry.Lookup("nagios")
	creator, native := collectorapi.DefaultRegistry.Lookup("native")
	require.True(t, nagios)
	require.True(t, native)
	require.NotNil(t, creator.CreateV2)
}
