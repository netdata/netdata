// SPDX-License-Identifier: GPL-3.0-or-later

package topologymodel

import (
	"testing"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/stretchr/testify/require"
)

func TestActorIsInferred(t *testing.T) {
	tests := map[string]struct {
		actor Actor
		want  bool
	}{
		"endpoint-type": {actor: Actor{ActorType: "endpoint"}, want: true},
		"inferred-detail": {
			actor: Actor{
				Detail: ActorDetail{
					L2: topologyengine.ProjectionActorDetail{
						Device: topologyengine.ProjectionDeviceActorDetail{Inferred: true},
					},
				},
			},
			want: true,
		},
		"device-type": {actor: Actor{ActorType: "device"}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, ActorIsInferred(tc.actor))
		})
	}
}
