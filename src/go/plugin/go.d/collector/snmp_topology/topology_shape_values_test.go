// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/stretchr/testify/require"
)

func TestTopologyActorIsInferred(t *testing.T) {
	tests := map[string]struct {
		actor topologyActor
		want  bool
	}{
		"endpoint-type": {actor: topologyActor{ActorType: "endpoint"}, want: true},
		"inferred-detail": {
			actor: topologyActor{
				Detail: topologyActorDetail{
					L2: topologyengine.ProjectionActorDetail{
						Device: topologyengine.ProjectionDeviceActorDetail{Inferred: true},
					},
				},
			},
			want: true,
		},
		"device-type": {actor: topologyActor{ActorType: "device"}},
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
