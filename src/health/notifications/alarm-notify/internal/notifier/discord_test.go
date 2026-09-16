// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderDiscord(t *testing.T) {
	full := expectedEvent()
	full.URL = "https://example.com/alert?id=1&view=chart#details"
	critical, recovery := full, full
	critical.Status = "CRITICAL"
	recovery.Status, recovery.PreviousStatus = "CLEAR", "CRITICAL"
	minimal := notifyevent.Event{Node: "node", Alert: "alert", Status: "WARNING", Summary: "summary", Timestamp: full.Timestamp}
	blank := minimal
	blank.Chart, blank.Context, blank.Info = " \n", "\t", " \n\t"
	tests := map[string]struct {
		event        notifyevent.Event
		fixture      string
		replacements []string
	}{
		"warning full": {event: full, fixture: "discord-full.json"},
		"critical full": {
			event: critical, fixture: "discord-full.json",
			replacements: []string{"WARNING", "CRITICAL", "16426522", "15548997"},
		},
		"recovery full": {
			event: recovery, fixture: "discord-full.json",
			replacements: []string{"CLEAR → WARNING", "CRITICAL → CLEAR", "WARNING", "CLEAR", "16426522", "5763719"},
		},
		"minimal unknown values": {event: minimal, fixture: "discord-minimal.json"},
		"blank optional details": {event: blank, fixture: "discord-minimal.json"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := renderDiscord(test.event)
			require.NoError(t, err)
			payload, err := json.Marshal(got)
			require.NoError(t, err)
			want, err := os.ReadFile(filepath.Join("testdata", test.fixture))
			require.NoError(t, err)
			assert.JSONEq(t, strings.NewReplacer(test.replacements...).Replace(string(want)), string(payload))
		})
	}
}

func TestDiscordText(t *testing.T) {
	tests := map[string]struct{ input, want string }{
		"ordinary Unicode": {input: "Unicode θερμοκρασία 界", want: "Unicode θερμοκρασία 界"},
		"formatting and mentions": {
			input: "*bold* _italic_ ~~strike~~ ||spoiler|| <@123> @everyone",
			want:  `\*bold\* \_italic\_ \~\~strike\~\~ \|\|spoiler\|\| \<@123\> @everyone`,
		},
		"links":              {input: "[name](https://example.com)", want: `\[name\]\(https://example\.com\)`},
		"code and backslash": {input: "`code` \\*text*", want: "\\`code\\` \\\\\\*text\\*"},
		"block formatting": {
			input: "# heading\n> quote\n- item\n1. item",
			want:  "\\# heading\n\\> quote\n\\- item\n1\\. item",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := discordText(test.input, 1024)
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestRenderDiscordLimits(t *testing.T) {
	tests := map[string]struct {
		field string
		value string
		err   string
	}{
		"title boundary": {field: "summary", value: strings.Repeat("x", 256-len("WARNING: "))},
		"title overflow": {
			field: "summary",
			value: strings.Repeat("x", 257-len("WARNING: ")),
			err:   "256-character",
		},
		"description boundary":   {field: "info", value: strings.Repeat("x", 4096)},
		"description overflow":   {field: "info", value: strings.Repeat("x", 4097), err: "4096-character"},
		"field Unicode boundary": {field: "node", value: strings.Repeat("界", 1024)},
		"non-BMP boundary":       {field: "node", value: strings.Repeat("😀", 1024)},
		"non-BMP overflow":       {field: "node", value: strings.Repeat("😀", 1025), err: "1024-character"},
		"field overflow":         {field: "node", value: strings.Repeat("x", 1025), err: "1024-character"},
		"escaped boundary":       {field: "node", value: strings.Repeat("*", 512)},
		"escaped overflow":       {field: "node", value: strings.Repeat("*", 513), err: "1024-character"},
		"aggregate boundary":     {field: "aggregate", value: strings.Repeat("a", 842)},
		"aggregate overflow":     {field: "aggregate", value: strings.Repeat("a", 843), err: "6000-character combined"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			event := notifyevent.Event{Node: "node", Alert: "alert", Status: "WARNING", Summary: "summary", Timestamp: time.Now()}
			switch test.field {
			case "summary":
				event.Summary = test.value
			case "info":
				event.Info = test.value
			case "node":
				event.Node = test.value
			case "aggregate":
				event.Node, event.Info = strings.Repeat("n", 1024), strings.Repeat("i", 4096)
				event.Alert = test.value
			}
			got, err := renderDiscord(event)
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.Equal(t, discordMessage{}, got)
				assert.NotContains(t, err.Error(), test.value)
				return
			}
			require.NoError(t, err)
			assert.Len(t, got.Embeds, 1)
		})
	}
}

func TestDiscordUsername(t *testing.T) {
	tests := map[string]struct{ node, want string }{
		"boundary":  {node: strings.Repeat("n", 21), want: "netdata on " + strings.Repeat("n", 21)},
		"truncated": {node: strings.Repeat("n", 22), want: "netdata on " + strings.Repeat("n", 18) + "..."},
		"Unicode":   {node: strings.Repeat("界", 22), want: "netdata on " + strings.Repeat("界", 18) + "..."},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			event := expectedEvent()
			event.Node = test.node
			got, err := renderDiscord(event)
			require.NoError(t, err)
			assert.Equal(t, test.want, got.Username)
			assert.Equal(t, test.node, got.Embeds[0].Fields[0].Value, "full node identity is retained")
		})
	}
}

func TestDiscordEndpoint(t *testing.T) {
	tests := map[string]struct{ endpoint, want, err string }{
		"default confirmed": {endpoint: "https://example.com/hook", want: "https://example.com/hook?wait=true"},
		"override wait and retain query": {
			endpoint: "https://example.com/hook?wait=false&thread_id=123&value=a%26b&value=c",
			want:     "https://example.com/hook?thread_id=123&value=a%26b&value=c&wait=true",
		},
		"duplicate wait replaced": {
			endpoint: "https://example.com/hook?wait=false&wait=false",
			want:     "https://example.com/hook?wait=true",
		},
		"bad query escape":         {endpoint: "https://example.com/hook?private=%zz", err: "URL query"},
		"query separator rejected": {endpoint: "https://example.com/hook?private=a;b", err: "URL query"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := discordEndpoint(test.endpoint)
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.Empty(t, got)
				assert.NotContains(t, err.Error(), "private")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}
