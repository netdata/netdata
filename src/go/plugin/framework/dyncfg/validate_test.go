// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/stretchr/testify/require"
)

func TestProtocolFieldCompatibilityWrappers(t *testing.T) {
	values := []string{
		"source",
		`C:\Program Files\Netdata`,
		`file=C:\`,
		"operator's",
		"line\nbreak",
	}
	for _, value := range values {
		require.Equal(t, netdataapi.ValidBareProtocolField(value), ValidBareProtocolField(value), value)
		require.Equal(t, netdataapi.ValidSingleQuotedProtocolField(value), ValidSingleQuotedProtocolField(value), value)
	}
}
