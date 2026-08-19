// SPDX-License-Identifier: GPL-3.0-or-later

package topologymodel

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalActorMatchIndexPreservesLocalIdentitySemantics(t *testing.T) {
	index := NewLocalActorMatchIndex()
	index.AddActorID(" endpoint-a ")
	index.AddMatch(0, Match{
		ChassisIDs: []string{" chassis-a ", "CHASSIS-A"},
		SysName:    "shared-name",
		IPAddresses: []string{
			"not-an-ip",
			"192.0.2.1",
			"192.0.2.1",
		},
	})
	index.AddActorID("router-b")
	index.AddMatch(1, Match{
		ChassisIDs:  []string{"chassis-b"},
		SysName:     " SHARED-NAME ",
		IPAddresses: []string{"192.0.2.2", "192.0.2.1"},
	})
	index.AddMatch(2, Match{ChassisIDs: []string{"chassis-c"}, IPAddresses: []string{"192.0.2.3"}})
	index.AddMatch(3, Match{SysName: "Σ"})

	tests := map[string]struct {
		local     Device
		wantFirst int
		wantAll   []int
	}{
		"chassis-case-and-space": {
			local:     Device{ChassisID: " CHASSIS-A "},
			wantFirst: 0,
			wantAll:   []int{0},
		},
		"same-actor-through-multiple-keys": {
			local:     Device{ChassisID: "chassis-b", SysName: "shared-name", ManagementIP: "192.0.2.1"},
			wantFirst: 0,
			wantAll:   []int{0, 1},
		},
		"selected-ip-only": {
			local:     Device{ManagementIP: "::ffff:192.0.2.3"},
			wantFirst: 2,
			wantAll:   []int{2},
		},
		"unicode-simple-fold": {
			local:     Device{SysName: "ς"},
			wantFirst: 3,
			wantAll:   []int{3},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			first, ok := index.FirstMatch(tc.local)
			require.True(t, ok)
			require.Equal(t, tc.wantFirst, first)
			require.Equal(t, tc.wantAll, index.MatchIndexes(nil, tc.local))
		})
	}

	_, ok := index.FirstMatch(Device{ManagementIP: "not-an-ip"})
	require.False(t, ok)
	require.True(t, index.ContainsActorID("endpoint-a"))
	require.False(t, index.ContainsActorID("ENDPOINT-A"))
}

func TestLocalActorMatchIndexAcceptsAppendedActor(t *testing.T) {
	index := NewLocalActorMatchIndex()
	index.AddActorID("router-a")
	index.AddMatch(0, Match{IPAddresses: []string{"192.0.2.1"}})
	index.AddActorID("router-b")
	index.AddMatch(1, Match{IPAddresses: []string{"192.0.2.2"}})

	position, ok := index.FirstMatch(Device{ManagementIP: "192.0.2.2"})
	require.True(t, ok)
	require.Equal(t, 1, position)
	require.True(t, index.ContainsActorID("router-b"))
}
