// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/require"
)

func TestDigestLayoutGolden(t *testing.T) {
	publication, err := DigestSortedPublications([]PublicationRecord{{
		Name: "module:method", Generation: 7, Timeout: 60,
		Help: "method help", Tags: "top", Access: "0x0013",
		Priority: 100, Version: 3,
		AvailabilityDigest: [32]byte{1, 2, 3, 4},
	}})
	require.NoError(t, err)

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

	require.EqualValues(
		t,
		"9b5257a6959e39654ccdcd386a5aa122b66fed051979376bcfeaa03eb6077c46",
		fmt.Sprintf("%x", publication),
	)
	require.EqualValues(
		t,
		"059a7744aacc08166569b6f9546c82e8d9774d09c6b0b29c36ba55ec8f37d2cd",
		controller,
	)
}
