// SPDX-License-Identifier: GPL-3.0-or-later

package msteams

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderTeamsMatrix(t *testing.T) {
	for provider := range map[string]struct{}{"msteams": {}} {
		for status := range map[string]struct{}{"WARNING": {}, "CRITICAL": {}, "CLEAR": {}} {
			for variant := range map[string]struct{}{"full": {}, "minimal": {}} {
				t.Run(provider+"/"+status+"/"+variant, func(t *testing.T) {
					event := testutil.EventForStatus(status, variant)
					message := renderMSTeams(Config{}, event)
					data, err := json.Marshal(message)
					require.NoError(t, err)
					assert.JSONEq(t, testutil.ReadStatusFixture(t, "testdata", provider, status, variant), string(data))
				})
			}
		}
	}
}

func TestTeamsMatrixEscaping(t *testing.T) {
	event := testutil.EventForStatus("CLEAR", "minimal")
	event.Summary = "[x](bad) *hot* <b>&\"quote\"</b>\n# next \\ end"
	event.URL = "https://example.com/a(b)<c>?q=\"x\"&y=1"
	assert.Equal(t, msTeamsMessage{
		Context: "http://schema.org/extensions", Type: "MessageCard", ThemeColor: "65A677", Title: "💚 Alert CLEAR from Netdata on node",
		Text: "\\[x\\]\\(bad\\) \\*hot\\* \\<b\\>\\&\"quote\"\\</b\\>  \n\\# next \\\\ end  \nNode: node  \nAlert: alert  \nStatus: CLEAR  \nTime: 2026\\-09\\-14T12:00:00Z\n\n[View alert](https://example.com/a%28b%29%3Cc%3E?q=%22x%22&y=1)",
	}, renderMSTeams(Config{}, event))
}

func TestMSTeamsStyle(t *testing.T) {
	for name, test := range map[string]struct {
		icons, colors  map[string]string
		prefix, color  string
		node, nodeText string
	}{
		"defaults": {prefix: "⚠️ ", color: "FFA500"}, "custom": {icons: map[string]string{"warning": "*alarm*"}, colors: map[string]string{"warning": "abcDEF"}, prefix: "*alarm* ", color: "abcDEF"},
		"empty overrides":      {icons: map[string]string{"warning": ""}, colors: map[string]string{"warning": ""}},
		"other status":         {icons: map[string]string{"clear": "x"}, colors: map[string]string{"critical": "abcdef"}, prefix: "⚠️ ", color: "FFA500"},
		"hostname punctuation": {node: "prod-db-01.example.net", nodeText: `prod\-db\-01\.example\.net`, prefix: "⚠️ ", color: "FFA500"},
		"literal Markdown":     {node: "db_*.[east]", nodeText: `db\_\*\.\[east\]`, prefix: "⚠️ ", color: "FFA500"},
		"literal backslash":    {node: `node\branch`, nodeText: `node\\branch`, prefix: "⚠️ ", color: "FFA500"},
		"Unicode hostname":     {node: "δοκιμή-01", nodeText: `δοκιμή\-01`, prefix: "⚠️ ", color: "FFA500"},
	} {
		t.Run(name, func(t *testing.T) {
			dst := Config{}
			dst.Icons, dst.Colors = test.icons, test.colors
			event := testutil.EventForStatus("WARNING", "minimal")
			var want msTeamsMessage
			require.NoError(
				t,
				json.Unmarshal([]byte(testutil.ReadStatusFixture(t, "testdata", "msteams", "WARNING", "minimal")), &want),
			)
			if test.node != "" {
				event.Node = test.node
				want.Text = strings.Replace(want.Text, "Node: node", "Node: "+test.nodeText, 1)
			}
			want.Title, want.ThemeColor = test.prefix+"Alert WARNING from Netdata on "+event.Node, test.color
			assert.Equal(t, want, renderMSTeams(dst, event))
		})
	}
}
