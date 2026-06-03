// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSNMPTopologyMethodConfigDoesNotUseLegacyPresentation(t *testing.T) {
	method := topologyMethodConfig()

	require.Equal(t, topologyMethodID, method.ID)
	require.Equal(t, "topology", method.ResponseType)
	require.Nil(t, method.Presentation())
}
