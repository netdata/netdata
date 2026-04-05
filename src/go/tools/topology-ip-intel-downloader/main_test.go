// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSourceDisplayNameRedactsURL(t *testing.T) {
	name := sourceDisplayName(sourceEntry{
		family:   sourceFamilyGeo,
		provider: providerDBIP,
		artifact: artifactDBIPCityLite,
		format:   formatMMDB,
		url:      "https://user:secret@example.test/download/city.mmdb.gz?token=abc#frag",
	})

	require.Equal(t, "dbip:city-lite@mmdb url=https://example.test/<redacted>", name)
}
