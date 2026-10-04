// SPDX-License-Identifier: GPL-3.0-or-later
package secrets

import (
	"github.com/stretchr/testify/require"
	"net/url"
	"testing"
)

func TestURLCredentialParameters(t *testing.T) {
	raw := "https://example.org/?client_secret=first&client%5Fsecret=second&CLIENT_SECRET=third&view=home&token=fourth"
	redacted := NewRedactor().ApplyURL(raw)
	for _, secret := range []string{"first", "second", "third", "fourth"} {
		require.NotContains(t, redacted, secret)
	}
	parsed, err := url.Parse(redacted)
	require.NoError(t, err)
	require.Equal(t, []string{"[REDACTED]", "[REDACTED]"}, parsed.Query()["client_secret"])
	require.Equal(t, "home", parsed.Query().Get("view"))
	require.Equal(t, "[REDACTED]", parsed.Query().Get("CLIENT_SECRET"))
}
