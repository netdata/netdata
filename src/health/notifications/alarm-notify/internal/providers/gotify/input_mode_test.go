// SPDX-License-Identifier: GPL-3.0-or-later

package gotify

import (
	"net/http"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInputModeTokenValidation(t *testing.T) {
	t.Setenv("NOTIFIER_GOTIFY_TOKEN", "valid-token")
	for name, test := range map[string]struct {
		mode    secret.InputMode
		token   string
		invalid bool
	}{
		"native reference deferred":       {token: "${env:NOTIFIER_GOTIFY_TOKEN}"},
		"literal env reference rejected":  {mode: secret.LiteralInput, token: "${env:NOTIFIER_GOTIFY_TOKEN}", invalid: true},
		"literal file reference rejected": {mode: secret.LiteralInput, token: "${file:/notifier/token}", invalid: true},
		"literal valid token":             {mode: secret.LiteralInput, token: "valid-token"},
		"literal whitespace rejected":     {mode: secret.LiteralInput, token: "invalid token", invalid: true},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := Config{Secrets: test.mode, APIURL: "https://example.com", AppToken: test.token}
			sender, err := New(cfg, http.DefaultClient)
			if test.invalid {
				require.EqualError(t, err, "gotify app_token must be nonempty printable ASCII without whitespace")
				assert.Nil(t, sender)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, cfg, sender.config)
		})
	}
}
