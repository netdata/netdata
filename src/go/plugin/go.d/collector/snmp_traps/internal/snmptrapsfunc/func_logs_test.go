// SPDX-License-Identifier: GPL-3.0-or-later

package snmptrapsfunc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogsFunctionConfigRequiresPaidPlan(t *testing.T) {
	// Viewing SNMP traps requires a paid plan (access 0x0017); reception and storage are not gated.
	cfg := LogsFunctionConfig(nil)
	assert.True(t, cfg.RequireCloud)
	assert.True(t, cfg.RequireCommercial)
}
