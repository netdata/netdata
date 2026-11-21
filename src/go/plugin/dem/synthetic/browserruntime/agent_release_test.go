// SPDX-License-Identifier: GPL-3.0-or-later

package browserruntime

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func testAgentRelease() AgentRelease {
	return AgentRelease{
		Version: "v2.12.0-607-nightly",
		Channel: "nightly",
		Source:  strings.Repeat("a", 40),
	}
}

func TestAgentReleaseIdentity(t *testing.T) {
	for _, version := range []string{"1", "v2.12.0", "v2.12.0-607-nightly", "0.custom_ref", strings.Repeat("A", 128)} {
		for _, channel := range []string{"stable", "release-candidate", "nightly", "devel"} {
			agent := testAgentRelease()
			agent.Version, agent.Channel = version, channel
			require.NoError(t, agent.Validate())
		}
	}
	for _, version := range []string{"", ".", "..", "-v2", "_v2", "../v2", "v2/path", "v2\\path", "v2+meta", "v2~rc", "v2%2f", "v2?key", "v2#key", "v2 value", "v2\n", "v2\x00", "v2é", strings.Repeat("a", 129)} {
		t.Run("version "+version, func(t *testing.T) {
			agent := testAgentRelease()
			agent.Version = version
			require.ErrorContains(t, agent.Validate(), "version")
		})
	}
	for _, channel := range []string{"", "edge", "preview", "rc", "Release-Candidate", "stable/other", "Nightly", "nightly\n"} {
		agent := testAgentRelease()
		agent.Channel = channel
		require.ErrorContains(t, agent.Validate(), "channel")
	}
	for _, source := range []string{"", "main", "abcdef0", strings.Repeat("a", 39), strings.Repeat("a", 41), strings.Repeat("A", 40), strings.Repeat("g", 40), strings.Repeat("a", 39) + "/"} {
		agent := testAgentRelease()
		agent.Source = source
		require.ErrorContains(t, agent.Validate(), "source")
	}
}
