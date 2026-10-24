// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/stretchr/testify/assert"
)

func TestNewPolicyCopiesOverrides(t *testing.T) {
	overrides := []Override{{
		OID:      "1.3.6.1.6.3.1.1.5.1",
		Category: "security",
		Labels:   map[string]string{"site": "dc-a"},
	}}
	policy := NewPolicy(PolicyConfig{Overrides: overrides})

	overrides[0].Category = "diagnostic"
	overrides[0].Labels["site"] = "dc-b"
	overrides[0].Labels["owner"] = "network"

	job := &Job{policy: policy}
	got := job.applyOverrides(&catalog.TrapDef{
		OID:      "1.3.6.1.6.3.1.1.5.1",
		Category: "state_change",
		Labels:   map[string]string{"source": "profile"},
	})

	assert.Equal(t, "security", got.Category)
	assert.Equal(t, map[string]string{"site": "dc-a", "source": "profile"}, got.Labels)
}
