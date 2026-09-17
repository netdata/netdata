// SPDX-License-Identifier: GPL-3.0-or-later

package legacydelivery

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/legacyconfig"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parsedPrograms(t *testing.T, texts ...string) []*legacyconfig.Program {
	t.Helper()
	var programs []*legacyconfig.Program
	for _, text := range texts {
		program, err := legacyconfig.Parse(strings.NewReader(text))
		require.NoError(t, err)
		programs = append(programs, program)
	}
	return programs
}

func TestPrepareSelection(t *testing.T) {
	const dynatrace = `SEND_DYNATRACE=YES; DYNATRACE_SPACE=space; DYNATRACE_SERVER=https://example.org; DYNATRACE_TOKEN=synthetic-private-value; DYNATRACE_TAG_VALUE=tag; DYNATRACE_EVENT=CUSTOM_INFO`
	tests := map[string]struct {
		config         []string
		methods, roles []string
		status         string
		history        *bool
		names          []string
		err            string
	}{
		"empty settings":                   {},
		"default exact YES":                {config: []string{`DISCORD_WEBHOOK_URL=https://example.org; DEFAULT_RECIPIENT_DISCORD=channel`}, names: []string{"discord-1"}},
		"disabled":                         {config: []string{`SEND_DISCORD=NO; DISCORD_WEBHOOK_URL=invalid; DEFAULT_RECIPIENT_DISCORD=channel`}},
		"case sensitive flag":              {config: []string{`SEND_DISCORD=yes; DISCORD_WEBHOOK_URL=invalid; DEFAULT_RECIPIENT_DISCORD=channel`}},
		"missing prerequisite":             {config: []string{`DEFAULT_RECIPIENT_DISCORD='channel|unknown'`}},
		"no recipients":                    {config: []string{`DISCORD_WEBHOOK_URL=invalid; curl_options=unsupported`}},
		"roles fallback and batch":         {config: []string{`DISCORD_WEBHOOK_URL=https://example.org; DEFAULT_RECIPIENT_DISCORD=default; role_recipients_discord[ops]='a b'; role_recipients_discord[empty]=''`}, roles: []string{"ops,empty unknown"}, names: []string{"discord-1"}},
		"whitespace suppresses":            {config: []string{`DISCORD_WEBHOOK_URL=https://example.org; DEFAULT_RECIPIENT_DISCORD=default; role_recipients_discord[ops]=' '`}},
		"reserved roles":                   {config: []string{`DISCORD_WEBHOOK_URL=invalid; DEFAULT_RECIPIENT_DISCORD=default`}, roles: []string{"silent disabled"}},
		"disabled recipient":               {config: []string{`DISCORD_WEBHOOK_URL=invalid; DEFAULT_RECIPIENT_DISCORD=disabled`}},
		"nowarn before invalid sender":     {config: []string{`DISCORD_WEBHOOK_URL=invalid; DEFAULT_RECIPIENT_DISCORD='channel|nowarn|critical'`}},
		"critical history missing":         {config: []string{`DISCORD_WEBHOOK_URL=https://example.org; DEFAULT_RECIPIENT_DISCORD='channel|critical'`}, err: "critical_seen_since_clear"},
		"critical no history needed":       {config: []string{`DISCORD_WEBHOOK_URL=https://example.org; DEFAULT_RECIPIENT_DISCORD='channel|critical'`}, status: "CRITICAL", names: []string{"discord-1"}},
		"critical unseen":                  {config: []string{`DISCORD_WEBHOOK_URL=invalid; DEFAULT_RECIPIENT_DISCORD='channel|critical'`}, history: new(false)},
		"critical seen":                    {config: []string{`DISCORD_WEBHOOK_URL=https://example.org; DEFAULT_RECIPIENT_DISCORD='channel|critical'`}, history: new(true), names: []string{"discord-1"}},
		"clear history":                    {config: []string{`DISCORD_WEBHOOK_URL=https://example.org; DEFAULT_RECIPIENT_DISCORD='channel|critical'`}, status: "CLEAR", history: new(true), names: []string{"discord-1"}},
		"permitted union":                  {config: []string{`DISCORD_WEBHOOK_URL=https://example.org; role_recipients_discord[ops]='channel|critical'; role_recipients_discord[all]=channel`}, roles: []string{"ops all"}, names: []string{"discord-1"}},
		"ordered overlays":                 {config: []string{`DISCORD_WEBHOOK_URL=invalid; DEFAULT_RECIPIENT_DISCORD=channel; role_recipients_discord[ops]="$DEFAULT_RECIPIENT_DISCORD"`, `DISCORD_WEBHOOK_URL=https://example.org; DEFAULT_RECIPIENT_DISCORD=disabled`}, names: []string{"discord-1"}},
		"map reset":                        {config: []string{`DISCORD_WEBHOOK_URL=https://example.org; role_recipients_discord[ops]=channel`, `role_recipients_discord=()`}},
		"selected subset":                  {config: []string{`DISCORD_WEBHOOK_URL=https://example.org; DEFAULT_RECIPIENT_DISCORD=channel; SLACK_WEBHOOK_URL=https://example.org; DEFAULT_RECIPIENT_SLACK=channel`}, methods: []string{"discord", "discord"}, names: []string{"discord-1"}},
		"unsupported alongside valid":      {config: []string{`DISCORD_WEBHOOK_URL=https://example.org; DEFAULT_RECIPIENT_DISCORD=channel; SEND_ILERT=YES; ILERT_ALERT_SOURCE_URL=https://example.org/synthetic-private-value`}, err: "legacy ilert"},
		"slack filtered":                   {config: []string{`SLACK_WEBHOOK_URL=https://example.org; DEFAULT_RECIPIENT_SLACK='channel|nowarn'`}},
		"inert custom function":            {config: []string{`custom_sender() { exit 123; }; DEFAULT_RECIPIENT_CUSTOM=''`}},
		"eligible custom":                  {config: []string{`role_recipients_custom[ops]=recipient; custom_sender() { exit 123; }`}, err: "custom_sender execution"},
		"explicit unmapped":                {methods: []string{"custom"}, err: "custom_sender execution"},
		"unknown method safe":              {methods: []string{"synthetic-private-value"}, err: "unknown legacy method"},
		"global ignores roles":             {config: []string{`KAFKA_URL=https://example.org; KAFKA_SENDER_IP=192.0.2.1; SEND_SIGNL4=YES; SIGNL4_WEBHOOK_URL=https://example.org`}, roles: []string{"silent"}, names: []string{"kafka-1", "signl4-1"}},
		"global disabled":                  {config: []string{`SEND_KAFKA=NO; KAFKA_URL=https://example.org; KAFKA_SENDER_IP=192.0.2.1`}},
		"signl4 not implicitly enabled":    {config: []string{`SIGNL4_WEBHOOK_URL=https://example.org`}},
		"opsgenie no recipients":           {config: []string{`SEND_OPSGENIE=YES; OPSGENIE_API_KEY=synthetic-private-value`}, names: []string{"opsgenie-1"}},
		"dynatrace no recipients":          {config: []string{dynatrace}, names: []string{"dynatrace-1"}},
		"dynatrace ignores silent role":    {config: []string{dynatrace}, roles: []string{"silent"}, names: []string{"dynatrace-1"}},
		"ilert not implicitly enabled":     {config: []string{`ILERT_INTEGRATION_KEY=synthetic-key; ILERT_ALERT_SOURCE_URL=ignored`}},
		"opsgenie not implicitly enabled":  {config: []string{`OPSGENIE_API_KEY=synthetic-key`}},
		"dynatrace not implicitly enabled": {config: []string{dynatrace, `SEND_DYNATRACE=''`}},
		"dynatrace disabled":               {config: []string{dynatrace, `SEND_DYNATRACE=NO`}},
		"dynatrace missing token":          {config: []string{dynatrace, `DYNATRACE_TOKEN=''`}},
		"old Teams aliases":                {config: []string{`SEND_MSTEAMS=NO; MSTEAM_WEBHOOK_URL=https://example.org; SEND_MSTEAM=YES; role_recipients_msteam[ops]=channel`}, names: []string{"msteams-1"}},
		"old Teams disabling alias":        {config: []string{`SEND_MSTEAM=NO; MSTEAMS_WEBHOOK_URL=https://example.org; DEFAULT_RECIPIENT_MSTEAMS=channel`}},
		"unknown modifier safe":            {config: []string{`DISCORD_WEBHOOK_URL=https://example.org; DEFAULT_RECIPIENT_DISCORD='synthetic-private-value|unknown'`}, err: "unknown recipient modifier"},
		"bad selected sender atomic":       {config: []string{`DISCORD_WEBHOOK_URL=https://example.org; DEFAULT_RECIPIENT_DISCORD=channel; GOTIFY_APP_URL=https://example.org; GOTIFY_APP_TOKEN='${env:SYNTHETIC_PRIVATE}'; DEFAULT_RECIPIENT_GOTIFY=channel`}, err: "gotify"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Setenv("PATH", t.TempDir())
			n := notifier.Notification{Event: testutil.ExpectedEvent(), CriticalSeenSinceClear: tt.history}
			if tt.status != "" {
				n.Event.Status = tt.status
			}
			roles := tt.roles
			if roles == nil {
				roles = []string{"ops"}
			}
			plan, names, err := Prepare(context.Background(), parsedPrograms(t, tt.config...), tt.methods, roles, n, http.DefaultClient, nil)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
				assert.Equal(t, notifier.Plan{}, plan)
				assert.Nil(t, names)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.names, names)
				assert.Len(t, plan.Destinations, len(tt.names))
				for _, name := range names {
					assert.NotNil(t, plan.Destinations[name])
				}
			}
		})
	}
}

func TestPrepareUnsupportedSettings(t *testing.T) {
	tests := map[string]struct{ setting, method, key string }{
		"curl path":         {`curl=/synthetic-private-value/curl`, "discord", "curl"},
		"curl options":      {`curl_options='--insecure synthetic-private-value'`, "discord", "curl_options"},
		"date format":       {`date_format='+synthetic-private-value'`, "discord", "date_format"},
		"images":            {`images_base_url=https://synthetic-private-value.invalid`, "discord", "images_base_url"},
		"fqdn":              {`use_fqdn=YES`, "discord", "use_fqdn"},
		"clear eligibility": {`clear_alarm_always=YES`, "discord", "clear_alarm_always"},
		"event mutation":    {`host=synthetic-private-value`, "discord", "host"},
		"non UTF email":     {`EMAIL_CHARSET=synthetic-private-value`, "email", "EMAIL_CHARSET"},
		"configured PATH":   {`PATH=/synthetic-private-value`, "sms", "PATH"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := `DISCORD_WEBHOOK_URL=https://example.org; DEFAULT_RECIPIENT_DISCORD=channel; sendmail=/fixture/sendmail; DEFAULT_RECIPIENT_EMAIL=root; sendsms=/fixture/sendsms; DEFAULT_RECIPIENT_SMS=123; ` + tt.setting
			n := notifier.Notification{Event: testutil.ExpectedEvent()}
			plan, names, err := Prepare(context.Background(), parsedPrograms(t, cfg), []string{tt.method}, []string{"ops"}, n, http.DefaultClient, nil)
			require.ErrorContains(t, err, "legacy setting "+tt.key+" is not supported")
			assert.NotContains(t, err.Error(), "synthetic-private-value")
			assert.Equal(t, notifier.Plan{}, plan)
			assert.Nil(t, names)
		})
	}
}

func TestPrepareCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	plan, names, err := Prepare(ctx, nil, nil, []string{"ops"}, notifier.Notification{Event: testutil.ExpectedEvent()}, http.DefaultClient, nil)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, notifier.Plan{}, plan)
	assert.Nil(t, names)
}
