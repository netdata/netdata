// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/assert"
)

func TestModuleRegistryWithSystemdPolicy(t *testing.T) {
	base := collectorapi.Registry{
		"logind": collectorapi.Creator{},
		"other":  collectorapi.Creator{},
	}

	withPolicy := moduleRegistryWithSystemdPolicy(base, 239)
	assert.True(t, withPolicy["logind"].Disabled)
	assert.False(t, withPolicy["other"].Disabled)
	assert.False(t, base["logind"].Disabled, "base registry must remain unchanged")

	withoutPolicy := moduleRegistryWithSystemdPolicy(base, 250)
	assert.False(t, withoutPolicy["logind"].Disabled)
	assert.False(t, withoutPolicy["other"].Disabled)
}
