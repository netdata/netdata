// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSNSARN = "arn:aws:sns:us-east-1:123456789012:alerts"

func snsDestination(t *testing.T) Destination {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	return Destination{Type: "awssns", Executable: executable, TargetARN: testSNSARN, CredentialSource: "imds"}
}

func TestSNSConfig(t *testing.T) {
	for name, test := range map[string]struct {
		change func(*Destination)
		err    string
	}{
		"imds": {},
		"static": {change: func(d *Destination) {
			d.CredentialSource = "static"
			d.Env = map[string]string{"AWS_ACCESS_KEY_ID": "${env:UNREAD_KEY}", "AWS_SECRET_ACCESS_KEY": "${file:/unread/secret}", "AWS_SESSION_TOKEN": "synthetic-private-value"}
		}},
		"web identity": {change: func(d *Destination) {
			d.CredentialSource = "web_identity"
			d.Env = map[string]string{"AWS_ROLE_ARN": "arn:aws:iam::123456789012:role/path/notifier", "AWS_WEB_IDENTITY_TOKEN_FILE": "/unread/token", "AWS_ROLE_SESSION_NAME": "notifier-session"}
		}},
		"web identity punctuation path": {change: func(d *Destination) {
			d.CredentialSource = "web_identity"
			d.Env = map[string]string{"AWS_ROLE_ARN": "arn:aws:iam::123456789012:role/team:ops!#$%&'()*;<>?[]^_`{|}~/notifier", "AWS_WEB_IDENTITY_TOKEN_FILE": "/unread/token"}
		}},
		"ecs": {change: func(d *Destination) {
			d.CredentialSource = "ecs"
			d.Env = map[string]string{"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI": "/v2/credentials/example?key=value"}
		}},
		"platform endpoint": {change: func(d *Destination) { d.TargetARN = "arn:aws:sns:eu-west-1:123456789012:endpoint/APNS/app/1234-5678" }},
		"china":             {change: func(d *Destination) { d.TargetARN = "arn:aws-cn:sns:cn-north-1:123456789012:alerts" }},
		"government":        {change: func(d *Destination) { d.TargetARN = "arn:aws-us-gov:sns:us-gov-west-1:123456789012:alerts" }},
		"message":           {change: func(d *Destination) { d.MessageTemplate = "{{ status }} {{node}}: {{summary}}\n{{info}}" }},
		"executable":        {change: func(d *Destination) { d.Executable = "aws" }, err: "absolute path"},
		"source required":   {change: func(d *Destination) { d.CredentialSource = "" }, err: "credential_source"},
		"missing static":    {change: func(d *Destination) { d.CredentialSource = "static" }, err: "required"},
		"unknown template":  {change: func(d *Destination) { d.MessageTemplate = "{{synthetic-private-value}}" }, err: "unknown placeholder"},
		"unclosed template": {change: func(d *Destination) { d.MessageTemplate = "{{node" }, err: "unclosed placeholder"},
		"template size":     {change: func(d *Destination) { d.MessageTemplate = strings.Repeat("x", snsMessageLimit+1) }, err: "262144"},
	} {
		t.Run(name, func(t *testing.T) {
			dst := snsDestination(t)
			if test.change != nil {
				test.change(&dst)
			}
			checkFormConfig(t, dst, test.err)
		})
	}
	for name, arn := range map[string]string{
		"empty": "", "reference": "${env:ARN}", "account": "arn:aws:sns:us-east-1:account:alerts",
		"region suffix": "arn:aws:sns:us-east-1.evil.example:123456789012:alerts", "service": "arn:aws:sqs:us-east-1:123456789012:alerts",
		"fifo": testSNSARN + ".fifo", "extra": testSNSARN + ":extra", "newline": testSNSARN + "\n",
	} {
		t.Run("arn/"+name, func(t *testing.T) {
			dst := snsDestination(t)
			dst.TargetARN = arn
			checkFormConfig(t, dst, "target_arn")
		})
	}
}

func TestSNSPlatformApplicationNames(t *testing.T) {
	for name, test := range map[string]struct {
		application string
		valid       bool
	}{
		"plain":                  {"app", true},
		"period":                 {"app.v1", true},
		"all allowed characters": {"App_1-v2.3", true},
		"one character":          {".", true},
		"256 characters":         {strings.Repeat("a", 253) + ".v1", true},
		"empty":                  {"", false},
		"257 characters":         {strings.Repeat("a", 257), false},
		"slash":                  {"app/name", false},
		"space":                  {"app name", false},
		"unicode":                {"app.é", false},
	} {
		t.Run(name, func(t *testing.T) {
			dst := snsDestination(t)
			dst.TargetARN = "arn:aws:sns:us-east-1:123456789012:endpoint/APNS/" + test.application + "/12345678-1234-1234-1234-123456789012"
			message := "target_arn"
			if test.valid {
				message = ""
			}
			checkFormConfig(t, dst, message)
		})
	}
	t.Run("topic periods remain invalid", func(t *testing.T) {
		dst := snsDestination(t)
		dst.TargetARN = testSNSARN + ".v1"
		checkFormConfig(t, dst, "target_arn")
	})
}

func TestSNSCredentialValues(t *testing.T) {
	for name, test := range map[string]struct{ source, key, value string }{
		"profile":        {"imds", "AWS_PROFILE", "synthetic-private-value"},
		"endpoint":       {"imds", "AWS_ENDPOINT_URL", "https://synthetic-private-value"},
		"full URI":       {"ecs", "AWS_CONTAINER_CREDENTIALS_FULL_URI", "http://localhost/"},
		"URI authority":  {"ecs", "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "@localhost/path"},
		"URI port":       {"ecs", "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", ":1234/path"},
		"URI fragment":   {"ecs", "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "/path#fragment"},
		"URI backslash":  {"ecs", "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "/path\\x"},
		"URI space":      {"ecs", "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "/path with space"},
		"URI escape":     {"ecs", "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "/%zz"},
		"role":           {"web_identity", "AWS_ROLE_ARN", "invalid"},
		"token path":     {"web_identity", "AWS_WEB_IDENTITY_TOKEN_FILE", "relative"},
		"session":        {"web_identity", "AWS_ROLE_SESSION_NAME", "invalid space"},
		"empty optional": {"static", "AWS_SESSION_TOKEN", ""},
		"NUL":            {"static", "AWS_SECRET_ACCESS_KEY", "synthetic-private-value\x00"},
		"control":        {"static", "AWS_SECRET_ACCESS_KEY", "synthetic-private-value\ninside"},
	} {
		t.Run(name, func(t *testing.T) {
			env := map[string]string{}
			switch test.source {
			case "static":
				env["AWS_ACCESS_KEY_ID"], env["AWS_SECRET_ACCESS_KEY"] = "key", "secret"
			case "web_identity":
				env["AWS_ROLE_ARN"], env["AWS_WEB_IDENTITY_TOKEN_FILE"] = "arn:aws:iam::123456789012:role/notifier", "/token"
			case "ecs":
				env["AWS_CONTAINER_CREDENTIALS_RELATIVE_URI"] = "/credentials"
			}
			env[test.key] = test.value
			for _, references := range []bool{true, false} {
				err := validateSNSCredentials(test.source, env, references)
				require.Error(t, err)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
			}
			// References defer value validation until selection and resolution.
			path := filepath.Join(t.TempDir(), "credential")
			require.NoError(t, os.WriteFile(path, []byte(test.value), 0600))
			env[test.key] = "${file:" + path + "}"
			dst := snsDestination(t)
			dst.CredentialSource, dst.Env = test.source, env
			_, err := snsEnvironment(context.Background(), dst, "us-east-1")
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "synthetic-private-value")
		})
	}
}

func TestSNSFieldIsolation(t *testing.T) {
	fields := reflect.TypeFor[Destination]()
	for i := 0; i < fields.NumField(); i++ {
		field := fields.Field(i)
		switch field.Name {
		case "Type", "Executable", "Env", "TargetARN", "CredentialSource", "MessageTemplate":
			continue
		}
		t.Run(field.Name, func(t *testing.T) {
			dst := snsDestination(t)
			v := reflect.ValueOf(&dst).Elem().Field(i)
			switch v.Kind() {
			case reflect.String:
				v.SetString("synthetic-private-value")
			case reflect.Slice:
				v.Set(reflect.ValueOf([]string{"synthetic-private-value"}))
			case reflect.Map:
				v.Set(reflect.ValueOf(map[string]string{"key": "synthetic-private-value"}))
			default:
				v.Set(reflect.ValueOf(new(configInteger(1))))
			}
			checkFormConfig(t, dst, "fields for another provider")
		})
	}
	for _, field := range []string{"TargetARN", "CredentialSource", "MessageTemplate"} {
		for _, provider := range []string{"webhook", "slack", "discord", "telegram", "pushover", "pushbullet", "twilio", "messagebird", "gotify", "ntfy", "rocketchat", "flock", "fleep", "ilert", "signl4", "alerta", "dynatrace", "prowl", "kavenegar", "smseagle", "pagerduty", "opsgenie", "msteams", "matrix", "command", "smstools3", "syslog"} {
			t.Run(provider+"/"+field, func(t *testing.T) {
				dst := Destination{Type: provider}
				reflect.ValueOf(&dst).Elem().FieldByName(field).SetString("synthetic-private-value")
				checkFormConfig(t, dst, "fields for another provider")
			})
		}
	}
}

func TestRenderSNS(t *testing.T) {
	for name, test := range map[string]struct {
		change                          func(*Event)
		template, subject, message, err string
	}{
		"warning":            {subject: "test-node needs attention - test alert - test.chart", message: "WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C"},
		"critical":           {change: func(e *Event) { e.Status = "CRITICAL" }, subject: "test-node is critical - test alert - test.chart", message: "CRITICAL on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C"},
		"clear zero":         {change: func(e *Event) { e.Status = "CLEAR"; e.Value = new(float64(0)) }, subject: "test-node recovered - test alert - test.chart", message: "CLEAR on test-node at 2026-09-14T12:00:00Z: test.chart 0 C"},
		"minimal":            {change: func(e *Event) { e.Chart, e.Units = "", ""; e.Value = nil }, subject: "test-node needs attention - test alert", message: "WARNING on test-node at 2026-09-14T12:00:00Z:"},
		"raw value":          {change: func(e *Event) { e.Units = "" }, subject: "test-node needs attention - test alert - test.chart", message: "WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 42.5"},
		"template":           {template: "{{status}} {{node}}\n{{summary}}: {{value_string}}", subject: "test-node needs attention - test alert - test.chart", message: "WARNING test-node\nTemperature is high: 42.5 C"},
		"literal input":      {template: "file://{{info}}", change: func(e *Event) { e.Info = "{{node}} $(literal) ${env:LITERAL}" }, subject: "test-node needs attention - test alert - test.chart", message: "file://{{node}} $(literal) ${env:LITERAL}"},
		"max message":        {template: "{{info}}", change: func(e *Event) { e.Info = strings.Repeat("é", snsMessageLimit/2) }, subject: "test-node needs attention - test alert - test.chart", message: strings.Repeat("é", snsMessageLimit/2)},
		"oversize expansion": {template: "{{info}}{{info}}", change: func(e *Event) { e.Info = strings.Repeat("é", snsMessageLimit/2) }, err: "exceeds"},
		"empty expansion":    {template: "{{info}}", change: func(e *Event) { e.Info = "" }, err: "nonempty"},
		"invalid UTF8":       {template: "{{info}}", change: func(e *Event) { e.Info = "\xff" }, err: "UTF-8"},
		"subject newline":    {change: func(e *Event) { e.Chart += "\n" }, err: "subject"},
		"subject NUL":        {change: func(e *Event) { e.Node += "\x00" }, err: "subject"},
		"subject separator":  {change: func(e *Event) { e.Alert += "\u2028" }, err: "subject"},
		"subject too long":   {change: func(e *Event) { e.Node = strings.Repeat("n", 100) }, err: "under 100"},
	} {
		t.Run(name, func(t *testing.T) {
			event := expectedEvent()
			if test.change != nil {
				test.change(&event)
			}
			got, err := renderSNS(Destination{TargetARN: testSNSARN, MessageTemplate: test.template}, event)
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, snsPublish{TargetARN: testSNSARN, Subject: test.subject, Message: test.message}, got)
		})
	}
}

func TestSNSTemplateFields(t *testing.T) {
	fields := snsFields(expectedEvent())
	assert.Equal(t, map[string]string{
		"version": "1", "incident_id": "test-incident", "timestamp": "2026-09-14T12:00:00Z", "node": "test-node",
		"alert": "test_alert", "chart": "test.chart", "context": "test.context", "status": "WARNING", "previous_status": "CLEAR",
		"summary": "Temperature is high", "info": "A quote: \"hot\"\nUnicode: θερμοκρασία", "value": "42.5", "previous_value": "0",
		"units": "C", "url": "", "status_message": "needs attention", "value_string": "42.5 C", "previous_value_string": "0 C",
		"duration": "", "non_clear_duration": "",
	}, fields)
	for name, test := range map[string]struct{ input, want, err string }{
		"escapes": {input: "{{{{node}} {{ node }} ${status} }}", want: "{{node}} test-node ${status} }}"},
		"unknown": {input: "{{secret}}", err: "unknown"}, "unclosed": {input: "prefix {{node", err: "unclosed"},
		"empty": {input: "{{}}", err: "unknown"}, "invalid UTF8": {input: "\xff", err: "UTF-8"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := renderSNSTemplate(test.input, fields)
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
	fields = snsFields(Event{})
	for _, key := range []string{"value", "previous_value", "value_string", "previous_value_string", "duration", "non_clear_duration"} {
		assert.Empty(t, fields[key])
	}
}

func TestSNSDurationTemplates(t *testing.T) {
	for name, test := range map[string]struct {
		duration, nonClear *uint32
		want               string
	}{
		"unknown":          {want: "previous=; non-clear="},
		"zero and nonzero": {duration: new(uint32(0)), nonClear: new(uint32(123)), want: "previous=0; non-clear=123"},
		"maximum":          {duration: new(uint32(4294967295)), want: "previous=4294967295; non-clear="},
	} {
		t.Run(name, func(t *testing.T) {
			event := expectedEvent()
			event.Duration, event.NonClearDuration = test.duration, test.nonClear
			got, err := renderSNS(Destination{TargetARN: testSNSARN, MessageTemplate: "previous={{duration}}; non-clear={{non_clear_duration}}"}, event)
			require.NoError(t, err)
			assert.Equal(t, snsPublish{TargetARN: testSNSARN, Subject: "test-node needs attention - test alert - test.chart", Message: test.want}, got)
		})
	}
}

func TestSNSSubjectBoundary(t *testing.T) {
	for name, test := range map[string]struct {
		extra int
		valid bool
	}{"99 characters": {75, true}, "100 characters": {76, false}} {
		t.Run(name, func(t *testing.T) {
			event := expectedEvent()
			event.Node, event.Chart, event.Alert = strings.Repeat("é", test.extra), "", "alert"
			_, err := renderSNS(Destination{}, event)
			if test.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestSNSReferencedCredentials(t *testing.T) {
	dst := snsDestination(t)
	dst.CredentialSource = "static"
	t.Setenv("SNS_SYNTHETIC_KEY", "key")
	path := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(path, []byte("secret\n"), 0600))
	dst.Env = map[string]string{"AWS_ACCESS_KEY_ID": "${env:SNS_SYNTHETIC_KEY}", "AWS_SECRET_ACCESS_KEY": "${file:" + path + "}"}
	env, err := snsEnvironment(context.Background(), dst, "us-east-1")
	require.NoError(t, err)
	assert.Contains(t, env, "AWS_ACCESS_KEY_ID=key")
	assert.Contains(t, env, "AWS_SECRET_ACCESS_KEY=secret")
}
