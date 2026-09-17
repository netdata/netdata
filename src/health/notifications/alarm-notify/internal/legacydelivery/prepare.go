// SPDX-License-Identifier: GPL-3.0-or-later

// Package legacydelivery maps evaluated shell-format settings to existing Go senders.
package legacydelivery

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/commandexec"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/legacyconfig"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/legacyrouting"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
)

type method struct {
	name     string
	required []string
	global   bool
	tool     string
	build    func(*builder, []string) ([]notifier.Sender, error)
	gap      string
}

type builder struct {
	values map[string]string
	client *http.Client
	runner *commandexec.Runner
}

// Order follows the legacy method inventory and its remaining global sends.
var methods = []method{
	{name: "alerta", required: []string{"ALERTA_WEBHOOK_URL"}, build: buildAlerta},
	{name: "awssns", tool: "aws", build: buildAWSSNS},
	{name: "custom", gap: "Unix custom_sender execution is not implemented"},
	{name: "discord", required: []string{"DISCORD_WEBHOOK_URL"}, build: buildDiscord},
	{name: "dynatrace", required: []string{"DYNATRACE_SPACE", "DYNATRACE_SERVER", "DYNATRACE_TOKEN", "DYNATRACE_TAG_VALUE", "DYNATRACE_EVENT"}, global: true, build: buildDynatrace},
	{name: "email", tool: "sendmail", build: buildEmail},
	{name: "fleep", required: []string{"FLEEP_SENDER"}, build: buildFleep},
	{name: "flock", required: []string{"FLOCK_WEBHOOK_URL"}, build: buildFlock},
	{name: "gotify", required: []string{"GOTIFY_APP_URL", "GOTIFY_APP_TOKEN"}, build: buildGotify},
	{name: "hipchat", required: []string{"HIPCHAT_AUTH_TOKEN"}, gap: "HipChat is excluded from the Go notifier"},
	{name: "irc", required: []string{"IRC_NETWORK"}, tool: "nc", build: buildIRC},
	{name: "kavenegar", required: []string{"KAVENEGAR_API_KEY", "KAVENEGAR_SENDER"}, build: buildKavenegar},
	{name: "matrix", required: []string{"MATRIX_HOMESERVER", "MATRIX_ACCESSTOKEN"}, build: buildMatrix},
	{name: "messagebird", required: []string{"MESSAGEBIRD_ACCESS_KEY", "MESSAGEBIRD_NUMBER"}, build: buildMessageBird},
	{name: "msteams", required: []string{"MSTEAMS_WEBHOOK_URL"}, build: buildMSTeams},
	{name: "ntfy", build: buildNtfy},
	{name: "pd", build: buildPagerDuty},
	{name: "prowl", build: buildProwl},
	{name: "pushbullet", required: []string{"PUSHBULLET_ACCESS_TOKEN"}, build: buildPushbullet},
	{name: "pushover", required: []string{"PUSHOVER_APP_TOKEN"}, build: buildPushover},
	{name: "rocketchat", required: []string{"ROCKETCHAT_WEBHOOK_URL"}, build: buildRocketChat},
	{name: "slack", required: []string{"SLACK_WEBHOOK_URL"}, build: buildSlack},
	{name: "sms", tool: "sendsms", build: buildSMS},
	{name: "syslog", tool: "logger", build: buildSyslog},
	{name: "telegram", required: []string{"TELEGRAM_BOT_TOKEN"}, build: buildTelegram},
	{name: "twilio", required: []string{"TWILIO_ACCOUNT_SID", "TWILIO_ACCOUNT_TOKEN", "TWILIO_NUMBER"}, build: buildTwilio},
	{name: "smseagle", required: []string{"SMSEAGLE_API_URL", "SMSEAGLE_API_ACCESSTOKEN", "SMSEAGLE_MSG_TYPE"}, build: buildSMSEagle},
	{name: "kafka", required: []string{"KAFKA_URL", "KAFKA_SENDER_IP"}, global: true, build: buildKafka},
	{name: "opsgenie", required: []string{"OPSGENIE_API_KEY"}, global: true, build: buildOpsgenie},
	{name: "ilert", required: []string{"ILERT_INTEGRATION_KEY"}, global: true, build: buildIlert},
	{name: "signl4", required: []string{"SIGNL4_WEBHOOK_URL"}, global: true, build: buildSIGNL4},
}

// Prepare evaluates files and preflights the complete selection without sending.
// Requested methods use Bash suffixes (pd, sms); an empty list selects all methods.
func Prepare(ctx context.Context, programs []*legacyconfig.Program, requested, roles []string, n notifier.Notification, client *http.Client, runner *commandexec.Runner) (notifier.Plan, []string, error) {
	fail := func(err error) (notifier.Plan, []string, error) { return notifier.Plan{}, nil, err }
	selected, err := selectMethods(requested)
	if err != nil {
		return fail(err)
	}
	facts := eventValues(n, roles)
	initial := initialValues(facts)
	settings, err := legacyconfig.Evaluate(initial, programs...)
	if err != nil {
		return fail(fmt.Errorf("legacy configuration: %w", err))
	}
	migrateTeams(&settings)
	b := &builder{values: settings.Variables, client: client, runner: runner}
	var applicable []method
	var routed []string
	for _, m := range selected {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		flag := b.values["SEND_"+strings.ToUpper(m.name)]
		if flag != "YES" && !(m.name == "email" && flag == "AUTO") {
			continue
		}
		// Check the retired setting before prerequisites can silently disable ilert.
		if m.name == "ilert" && b.values["ILERT_ALERT_SOURCE_URL"] != "" {
			return fail(errors.New("legacy ilert: remove or clear ILERT_ALERT_SOURCE_URL and configure ILERT_INTEGRATION_KEY from an API alert source; optionally set ILERT_API_URL"))
		}
		ready := true
		for _, key := range m.required {
			if b.values[key] == "" {
				ready = false
			}
		}
		if !ready {
			continue
		}
		// Tool availability disables a method before its recipient policies apply.
		// A configured PATH is checked later, only when a recipient is eligible.
		if m.tool != "" && b.values[m.tool] == "" && b.values["PATH"] == "" {
			path, lookupErr := exec.LookPath(m.tool)
			if lookupErr != nil {
				continue
			}
			b.values[m.tool] = path
		}
		applicable = append(applicable, m)
		if !m.global {
			routed = append(routed, m.name)
		}
	}
	targets, err := legacyrouting.Resolve(settings, routed, roles, n)
	if err != nil {
		return fail(err)
	}
	recipients := make(map[string][]string)
	for _, target := range targets {
		recipients[target.Method] = append(recipients[target.Method], target.Recipient)
	}
	var eligible []method
	for _, m := range applicable {
		if !m.global && len(recipients[m.name]) == 0 {
			continue
		}
		if m.gap != "" {
			return fail(fmt.Errorf("legacy %s: %s", m.name, m.gap))
		}
		if m.tool != "" {
			if b.values["PATH"] != "" {
				return fail(errors.New("legacy setting PATH is not supported for executable discovery; configure an absolute executable path"))
			}
			path := b.values[m.tool]
			if !filepath.IsAbs(path) {
				return fail(fmt.Errorf("legacy %s: executable must be an absolute path", m.name))
			}
			b.values[m.tool] = path
		}
		eligible = append(eligible, m)
	}
	if len(eligible) > 0 {
		if err := unsupportedSettings(b.values, facts, eligible); err != nil {
			return fail(err)
		}
	}
	plan := notifier.Plan{Destinations: make(map[string]notifier.Sender)}
	var names []string
	for _, m := range eligible {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		senders, err := m.build(b, recipients[m.name])
		if err != nil {
			return fail(fmt.Errorf("legacy %s: %w", m.name, err))
		}
		for i, sender := range senders {
			name := fmt.Sprintf("%s-%d", m.name, i+1)
			names = append(names, name)
			plan.Destinations[name] = sender
		}
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	return plan, names, nil
}

func selectMethods(requested []string) ([]method, error) {
	if len(requested) == 0 {
		return methods, nil
	}
	var selected []method
	seen := make(map[string]bool)
	for _, name := range requested {
		found := false
		for _, m := range methods {
			if m.name != name {
				continue
			}
			found = true
			if m.gap != "" {
				return nil, fmt.Errorf("legacy %s: %s", m.name, m.gap)
			}
			if !seen[name] {
				selected = append(selected, m)
				seen[name] = true
			}
			break
		}
		if !found {
			return nil, errors.New("unknown legacy method; use a supported Bash method name")
		}
	}
	return selected, nil
}

func each(recipients []string, construct func(string) (notifier.Sender, error)) ([]notifier.Sender, error) {
	senders := make([]notifier.Sender, 0, len(recipients))
	for _, recipient := range recipients {
		sender, err := construct(recipient)
		if err != nil {
			return nil, err
		}
		senders = append(senders, sender)
	}
	return senders, nil
}
func one(sender notifier.Sender, err error) ([]notifier.Sender, error) {
	if err != nil {
		return nil, err
	}
	return []notifier.Sender{sender}, nil
}

// Old singular Teams keys override plural keys after all files have loaded.
func migrateTeams(s *legacyconfig.Settings) {
	for key, value := range maps.Clone(s.Variables) {
		if value == "" {
			continue
		}
		switch {
		case key == "SEND_MSTEAM":
			s.Variables["SEND_MSTEAMS"] = value
		case key == "DEFAULT_RECIPIENT_MSTEAM":
			s.Variables["DEFAULT_RECIPIENT_MSTEAMS"] = value
		case strings.HasPrefix(key, "MSTEAM_"):
			s.Variables["MSTEAMS_"+strings.TrimPrefix(key, "MSTEAM_")] = value
		}
	}
	if len(s.Recipients["role_recipients_msteam"]) > 0 {
		if s.Recipients["role_recipients_msteams"] == nil {
			s.Recipients["role_recipients_msteams"] = make(map[string]string)
		}
		maps.Copy(s.Recipients["role_recipients_msteams"], s.Recipients["role_recipients_msteam"])
	}
}
