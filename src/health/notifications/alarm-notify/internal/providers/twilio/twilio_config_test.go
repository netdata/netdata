// SPDX-License-Identifier: GPL-3.0-or-later

package twilio

import (
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestTwilioConfiguration(t *testing.T) {
	base := Config{
		AccountSID: twilioTestSID,
		AuthToken:  "synthetic-private-value",
		From:       "+15005550006",
		To:         "+15005550009",
	}
	full, sender, channel, secrets := base, base, base, base
	full.APIURL = "http://localhost:8081/proxy/"
	sender.From = "Netdata Ops"
	channel.From, channel.To = "whatsapp:+15005550006", "whatsapp:+15005550009"
	secrets.AccountSID, secrets.AuthToken, secrets.APIURL = "${env:NOTIFIER_UNUSED_SID}", "${file:"+filepath.Join(
		t.TempDir(),
		"unread",
	)+"}", "${env:NOTIFIER_UNUSED_API}"
	for name, test := range map[string]struct {
		changes map[string]any
		want    Config
		err     string
	}{
		"minimal":             {want: base},
		"full":                {changes: map[string]any{"api_url": full.APIURL}, want: full},
		"sender ID":           {changes: map[string]any{"from": sender.From}, want: sender},
		"channel":             {changes: map[string]any{"from": channel.From, "to": channel.To}, want: channel},
		"unresolved secrets":  {changes: map[string]any{"account_sid": secrets.AccountSID, "auth_token": secrets.AuthToken, "api_url": secrets.APIURL}, want: secrets},
		"missing SID":         {changes: map[string]any{"account_sid": nil}, err: "account_sid"},
		"SID path injection":  {changes: map[string]any{"account_sid": twilioTestSID + "/../secret"}, err: "account_sid"},
		"invalid SID prefix":  {changes: map[string]any{"account_sid": "SK00000000000000000000000000000000"}, err: "account_sid"},
		"SID nonhex":          {changes: map[string]any{"account_sid": "ACZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ"}, err: "account_sid"},
		"SID too short":       {changes: map[string]any{"account_sid": "AC000"}, err: "account_sid"},
		"missing token":       {changes: map[string]any{"auth_token": nil}, err: "auth_token"},
		"token line break":    {changes: map[string]any{"auth_token": "synthetic-private-value\n"}, err: "auth_token"},
		"token whitespace":    {changes: map[string]any{"auth_token": "synthetic-private-value extra"}, err: "auth_token"},
		"token control":       {changes: map[string]any{"auth_token": "synthetic-private-value\x00"}, err: "auth_token"},
		"token unicode":       {changes: map[string]any{"auth_token": "界"}, err: "auth_token"},
		"interpolation":       {changes: map[string]any{"account_sid": "prefix${env:SID}"}, err: "secret reference"},
		"relative file":       {changes: map[string]any{"auth_token": "${file:relative}"}, err: "absolute"},
		"missing sender":      {changes: map[string]any{"from": nil}, err: "twilio from"},
		"missing recipient":   {changes: map[string]any{"to": nil}, err: "twilio to"},
		"sender control":      {changes: map[string]any{"from": "sender\n"}, err: "twilio from"},
		"sender whitespace":   {changes: map[string]any{"from": " sender"}, err: "twilio from"},
		"sender reference":    {changes: map[string]any{"from": "${env:FROM}"}, err: "twilio from"},
		"recipient reference": {changes: map[string]any{"to": "${env:TO}"}, err: "twilio to"},
		"recipient list":      {changes: map[string]any{"to": "+15005550009 +15005550006"}, err: "twilio to"},
		"API query":           {changes: map[string]any{"api_url": "https://example.com/?secret=value"}, err: "query"},
		"API empty query":     {changes: map[string]any{"api_url": "https://example.com/?"}, err: "query"},
		"API fragment":        {changes: map[string]any{"api_url": "https://example.com/#"}, err: "fragment"},
		"API userinfo":        {changes: map[string]any{"api_url": "https://user:synthetic-private-value@example.com"}, err: "user information"},
		"relative API":        {changes: map[string]any{"api_url": "/proxy"}, err: "absolute HTTP(S)"},
		"official plaintext":  {changes: map[string]any{"api_url": "http://API.TWILIO.COM./"}, err: "requires HTTPS"},
	} {
		t.Run(name, func(t *testing.T) {
			fields := map[string]any{
				"type":        "twilio",
				"account_sid": base.AccountSID,
				"auth_token":  base.AuthToken,
				"from":        base.From,
				"to":          base.To,
			}
			for key, value := range test.changes {
				fields[key] = value
			}
			input, err := yaml.Marshal(map[string]any{"version": 1, "destinations": map[string]any{"sms": fields}})
			require.NoError(t, err)
			got, err := readConfig(strings.NewReader(string(input)))
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				for _, private := range []string{base.AccountSID, base.AuthToken, base.From, base.To} {
					assert.NotContains(t, err.Error(), private)
				}
				assert.Equal(t, testutil.Document[Config]{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, testutil.Document[Config]{Version: 1, Destinations: map[string]Config{"sms": test.want}}, got)
		})
	}
}
