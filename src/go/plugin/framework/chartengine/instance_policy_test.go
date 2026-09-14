// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

func TestResolveInstanceLabelPolicyUsesPlannerSemantics(t *testing.T) {
	policy, err := ResolveInstanceLabelPolicy(&charttpl.Instances{
		ByLabels: []string{
			"region",
			" * ",
			"! host",
			"host",
			"region",
			"!zone",
		},
		OptionalByLabels: []string{"pid", "worker"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"region"}, policy.RequiredKeys)
	require.Equal(t, []string{"pid", "worker"}, policy.OptionalKeys)
	require.Equal(t, []string{"host", "zone"}, policy.ExcludedKeys)
	require.True(t, policy.IncludeAll)
}

func TestResolveInstanceLabelPolicyRejectsInvalidExclusion(t *testing.T) {
	_, err := ResolveInstanceLabelPolicy(&charttpl.Instances{ByLabels: []string{"! "}})
	require.ErrorContains(t, err, "exclude token must include label key")
}
