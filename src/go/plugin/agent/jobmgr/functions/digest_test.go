// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/require"
)

func TestControllerGroupSignatureLayoutGolden(t *testing.T) {
	controller, err := controllerGroupSignature(
		methodGenerationAgent,
		[]funcapi.FunctionConfig{{
			ID: "method", FunctionName: "module:method", Name: "Method",
			UpdateEvery: 5, Help: "method help", RequireCloud: true,
			Tags: "top", ResponseType: "table", RawRequest: true,
			Aliases: []string{"method-alias"},
		}},
		nil,
	)
	require.NoError(t, err)

	require.EqualValues(t, "059a7744aacc08166569b6f9546c82e8d9774d09c6b0b29c36ba55ec8f37d2cd", controller)
}
