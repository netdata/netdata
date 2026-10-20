// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResourcePolicyIndependentCommand(t *testing.T) {
	policy := DynCfgJobResource(0, "go.d:collector:")
	policy.CommandArgument = 1
	policy.IndependentCommand = "restart"
	for _, command := range []string{"restart", "RESTART", "update", ""} {
		resource := policy.resolve([]string{"go.d:collector:module:job", command})
		if command == "restart" || command == "RESTART" {
			require.Empty(t, resource)
		} else {
			require.Equal(t, "module_job", resource)
		}
	}
	require.Equal(t, "module_job", policy.resolve([]string{"go.d:collector:module:job"}))
	require.NoError(t, policy.validate())
}
