// SPDX-License-Identifier: GPL-3.0-or-later

package snmpsd

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

func TestTargetHashIncludesTopologyRefreshEvery(t *testing.T) {
	cred := CredentialConfig{
		Name:      "public-v2",
		Version:   "2c",
		Community: "public",
	}
	sysInfo := snmputils.SysInfo{
		Descr:       "switch",
		Name:        "switch-a",
		SysObjectID: "1.3.6.1.4.1.9.1.1",
	}

	first := newTarget("192.0.2.10", cred, sysInfo, "30m")
	second := newTarget("192.0.2.10", cred, sysInfo, "45m")

	assert.NotEqual(t, first.Hash(), second.Hash())
	assert.NotEqual(t, first.TUID(), second.TUID())
}
