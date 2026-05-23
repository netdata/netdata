// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTopologyOutputStatHelpers_ClassifyActorsAndValues(t *testing.T) {
	require.True(t, topologyActorIsInferred(topologyActor{ActorType: "endpoint"}))
	require.True(t, topologyActorIsInferred(topologyActor{Labels: map[string]string{"inferred": "yes"}}))
	require.True(t, topologyActorIsInferred(topologyActor{Attributes: map[string]any{"inferred": true}}))
	require.False(t, topologyActorIsInferred(topologyActor{ActorType: "device"}))

	require.True(t, boolStatValue("true"))
	require.True(t, boolStatValue(" yes "))
	require.False(t, boolStatValue("0"))

	require.Equal(t, 7, intStatValue("7"))
	require.Equal(t, 5, intStatValue(int64(5)))
	require.Equal(t, 0, intStatValue("nan"))

	require.Equal(t, "value", topologyMetricValueString(map[string]any{"key": " value "}, "key"))
}
