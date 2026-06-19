// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTopologyActorIsInferred(t *testing.T) {
	tests := map[string]struct {
		actor topologyActor
		want  bool
	}{
		"endpoint-type":      {actor: topologyActor{ActorType: "endpoint"}, want: true},
		"inferred-label":     {actor: topologyActor{Labels: map[string]string{"inferred": "yes"}}, want: true},
		"inferred-attribute": {actor: topologyActor{Attributes: map[string]any{"inferred": true}}, want: true},
		"device-type":        {actor: topologyActor{ActorType: "device"}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, topologyActorIsInferred(tc.actor))
		})
	}
}

func TestBoolStatValue(t *testing.T) {
	tests := map[string]struct {
		in   any
		want bool
	}{
		"true":        {in: "true", want: true},
		"yes-trimmed": {in: " yes ", want: true},
		"zero":        {in: "0"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, boolStatValue(tc.in))
		})
	}
}

func TestIntStatValue(t *testing.T) {
	tests := map[string]struct {
		in   any
		want int
	}{
		"string": {in: "7", want: 7},
		"uint":   {in: uint(6), want: 6},
		"int64":  {in: int64(5), want: 5},
		"uint64": {in: uint64(30), want: 30},
		"nan":    {in: "nan"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, intStatValue(tc.in))
		})
	}
}

func TestTopologyMetricValueString(t *testing.T) {
	require.Equal(t, "value", topologyMetricValueString(map[string]any{"key": " value "}, "key"))
}
