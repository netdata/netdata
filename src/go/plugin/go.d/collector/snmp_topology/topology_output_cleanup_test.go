// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilterDanglingLinks_TrimActorIDsBeforeLookup(t *testing.T) {
	data := &topologyData{
		Actors: []topologyActor{
			{ActorID: "device-a"},
			{ActorID: "device-b"},
		},
		Links: []topologyLink{
			{SrcActorID: " device-a ", DstActorID: "\tdevice-b\n"},
			{SrcActorID: "device-a", DstActorID: "missing"},
		},
	}

	filterDanglingLinks(data)

	require.Len(t, data.Links, 1)
	require.Equal(t, " device-a ", data.Links[0].SrcActorID)
	require.Equal(t, "\tdevice-b\n", data.Links[0].DstActorID)
}
