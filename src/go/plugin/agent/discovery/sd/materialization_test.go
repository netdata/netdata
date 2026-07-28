// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMaterializationIdentityStructurallyEncodesValues(t *testing.T) {
	require.NotEqual(
		t,
		materializationIdentity("userconfig", []byte("fixture"), []byte("a\x00b"), []byte("c")),
		materializationIdentity("userconfig", []byte("fixture"), []byte("a"), []byte("b\x00c")),
	)
}
