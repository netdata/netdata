// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunLegacySNS(t *testing.T) {
	const defaultBody = "WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C"
	zero, missing, offset := testutil.ExpectedEvent(), testutil.ExpectedEvent(), testutil.ExpectedEvent()
	zero.Value, zero.PreviousValue = new(0.0), nil
	missing.Value, missing.PreviousValue = nil, nil
	offset.Timestamp = time.Date(2026, 9, 14, 15, 0, 0, 0, time.FixedZone("offset", 3*3600))
	for name, tt := range map[string]struct {
		source, status, arn, region, mode string
		env                               map[string]string
		configs                           []string
		producerContext                   string
		event                             event.Event
		want                              string
		code                              int
		stock                             bool
	}{
		"stock format initialized":    {stock: true, want: defaultBody},
		"unset format":                {want: defaultBody},
		"empty format":                {configs: []string{`AWSSNS_MESSAGE_FORMAT=''`}, want: defaultBody},
		"static literal credentials":  {source: "static", env: map[string]string{"AWS_ACCESS_KEY_ID": "${env:SNS_TEST_UNREAD}", "AWS_SECRET_ACCESS_KEY": "${file:/unread/synthetic-private-value}", "AWS_SESSION_TOKEN": "synthetic-private-value"}, want: defaultBody},
		"web identity critical":       {source: "web_identity", status: "CRITICAL", region: "eu-west-1", arn: "arn:aws:sns:eu-west-1:123456789012:alerts", env: map[string]string{"AWS_ROLE_ARN": "arn:aws:iam::123456789012:role/notifier", "AWS_WEB_IDENTITY_TOKEN_FILE": "/unread/token", "AWS_ROLE_SESSION_NAME": "notifier-test"}, configs: []string{`AWSSNS_MESSAGE_FORMAT="$status_message: $value_string (was $old_value_string)"`}, want: "is critical: 42.5 C (was 0 C)"},
		"ecs clear":                   {source: "ecs", status: "CLEAR", env: map[string]string{"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI": "/credentials"}, configs: []string{`AWSSNS_MESSAGE_FORMAT="$status_message: $date"`}, want: "recovered: 2026-09-14T12:00:00Z"},
		"zero and absent":             {event: zero, configs: []string{`AWSSNS_MESSAGE_FORMAT="[$value_string][$old_value_string]"`}, want: "[0 C][]"},
		"missing values":              {event: missing, configs: []string{`AWSSNS_MESSAGE_FORMAT="[$value_string][$old_value_string]"`}, want: "[][]"},
		"UTC date":                    {event: offset, configs: []string{`AWSSNS_MESSAGE_FORMAT="$date"`}, want: "2026-09-14T12:00:00Z"},
		"assignment order":            {configs: []string{`PREFIX=first; AWSSNS_MESSAGE_FORMAT="$PREFIX $status_message"`, `PREFIX=second`}, want: "first needs attention"},
		"last assignment":             {configs: []string{`AWSSNS_MESSAGE_FORMAT=first`, `AWSSNS_MESSAGE_FORMAT="$status_message"`}, want: "needs attention"},
		"literal syntax":              {configs: []string{`AWSSNS_MESSAGE_FORMAT='file://{{unknown}} ${date} $(literal) θερμοκρασία'`}, want: "file://{{unknown}} ${date} $(literal) θερμοκρασία"},
		"producer facts":              {producerContext: testutil.ProducerContextJSON, configs: []string{`AWSSNS_MESSAGE_FORMAT="$unique_id/$alarm_id/$event_id: $value_string ($old_value_string); warnings=$total_warnings critical=$total_critical; $calc_expression; $src"`}, want: "42/7/3: 42.50 °C (0.00 °C); warnings=2 critical=0; $this > 40; line=12,file=/etc/netdata/health.d/example.conf"},
		"producer text stays literal": {producerContext: `{"calc_param_values":"$(literal) ${env:UNREAD} {{unknown}}"}`, configs: []string{`AWSSNS_MESSAGE_FORMAT="$calc_param_values"`}, want: "$(literal) ${env:UNREAD} {{unknown}}"},
		"producer empty formatting":   {producerContext: `{"value_string":"","old_value_string":""}`, configs: []string{`AWSSNS_MESSAGE_FORMAT="[$value_string][$old_value_string]"`}, want: "[][]"},
		"event text is data":          {configs: []string{`AWSSNS_MESSAGE_FORMAT="$info"`}, want: testutil.ExpectedEvent().Info},
		"maximum literal braces":      {configs: []string{"AWSSNS_MESSAGE_FORMAT='" + strings.Repeat("{{", 131072) + "'"}, want: strings.Repeat("{{", 131072)},
		"command failure":             {mode: "-fail", code: 1, want: defaultBody},
	} {
		t.Run(name, func(t *testing.T) {
			for _, key := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_PROFILE", "AWS_ENDPOINT_URL", "AWS_ENDPOINT_URL_SNS", "AWS_CONFIG_FILE", "AWS_SHARED_CREDENTIALS_FILE", "AWS_CONTAINER_CREDENTIALS_FULL_URI", "AWS_EC2_METADATA_SERVICE_ENDPOINT", "HTTPS_PROXY", "BASH_ENV", "SNS_TEST_UNREAD"} {
				t.Setenv(key, "ambient-private-value")
			}
			dst, capture := snsHelperDestination(t, tt.mode)
			if tt.source == "" {
				tt.source = "imds"
			}
			if tt.arn == "" {
				tt.arn = testSNSARN
			}
			if tt.region == "" {
				tt.region = "us-east-1"
			}
			e := tt.event
			if e.Version == 0 {
				e = testutil.ExpectedEvent()
			}
			if tt.status != "" {
				e.Status = tt.status
			}
			input, err := json.Marshal(e)
			require.NoError(t, err)
			text := fmt.Sprintf("aws='%s'; AWSSNS_CREDENTIAL_SOURCE=%s; DEFAULT_RECIPIENT_AWSSNS='%s'", dst["executable"], tt.source, tt.arn)
			for key, value := range tt.env {
				text += fmt.Sprintf("; %s='%s'", key, value)
			}
			args := []string{"send-legacy", "--method", "awssns", "--role", "ops"}
			if tt.stock {
				args = append(args, "--config", filepath.Join("..", "..", "..", "health_alarm_notify.conf"))
			}
			for _, config := range append([]string{text}, tt.configs...) {
				args = append(args, "--config", writeConfig(t, config))
			}
			var stdout, stderr bytes.Buffer
			assert.Equal(t, tt.code, Run(context.Background(), args, strings.NewReader(testutil.WithProducerContext(string(input), tt.producerContext)), &stdout, &stderr), stderr.String())
			assert.Empty(t, stdout.String())
			assert.NotContains(t, stderr.String(), "private-value")
			captures := readCommandCaptures(t, capture)
			require.Len(t, captures, 1)
			assert.Equal(t, expectedSNSCapture(t, captures[0], tt.source, tt.region, tt.arn, e.Status, tt.want, tt.env), captures[0])
			assertSNSHomeRemoved(t, captures[0])
		})
	}
}

func TestRunLegacySNSRouting(t *testing.T) {
	const secondARN = "arn:aws:sns:eu-west-1:123456789012:other"
	for name, tt := range map[string]struct {
		overlay, input, executable string
		want                       []string
		code                       int
		err                        string
	}{
		"default target": {want: []string{testSNSARN}},
		"role overrides default and deduplicates": {overlay: "role_recipients_awssns[ops]='" + secondARN + " " + secondARN + " " + testSNSARN + "'", want: []string{secondARN, testSNSARN}},
		"disabled":                          {overlay: "SEND_AWSSNS=NO; AWSSNS_CREDENTIAL_SOURCE=invalid"},
		"case sensitive flag":               {overlay: "SEND_AWSSNS=yes"},
		"suppressed":                        {overlay: "role_recipients_awssns[ops]=disabled; AWSSNS_CREDENTIAL_SOURCE=invalid"},
		"nowarn":                            {overlay: "DEFAULT_RECIPIENT_AWSSNS='" + testSNSARN + "|nowarn|critical'; AWSSNS_CREDENTIAL_SOURCE=invalid"},
		"history unseen":                    {overlay: "DEFAULT_RECIPIENT_AWSSNS='" + testSNSARN + "|critical'; AWSSNS_CREDENTIAL_SOURCE=invalid", input: strings.Replace(testutil.ValidEvent, `"version": 1`, `"version": 1, "critical_seen_since_clear": false`, 1)},
		"history seen":                      {overlay: "DEFAULT_RECIPIENT_AWSSNS='" + testSNSARN + "|critical'", input: strings.Replace(testutil.ValidEvent, `"version": 1`, `"version": 1, "critical_seen_since_clear": true`, 1), want: []string{testSNSARN}},
		"history missing":                   {overlay: "DEFAULT_RECIPIENT_AWSSNS='" + testSNSARN + "|critical'", code: 1, err: "critical_seen_since_clear"},
		"discovered":                        {executable: "discovered", want: []string{testSNSARN}},
		"missing discovery before policies": {executable: "missing", overlay: "DEFAULT_RECIPIENT_AWSSNS='" + testSNSARN + "|critical'; AWSSNS_CREDENTIAL_SOURCE=''"},
		"no recipients":                     {overlay: "DEFAULT_RECIPIENT_AWSSNS=''; AWSSNS_CREDENTIAL_SOURCE=''"},
	} {
		t.Run(name, func(t *testing.T) {
			dst, capture := snsHelperDestination(t, "")
			executable := dst["executable"].(string)
			switch tt.executable {
			case "discovered":
				t.Setenv("PATH", filepath.Dir(executable))
				executable = ""
			case "missing":
				t.Setenv("PATH", t.TempDir())
				executable = ""
			}
			text := fmt.Sprintf("aws='%s'; AWSSNS_CREDENTIAL_SOURCE=imds; DEFAULT_RECIPIENT_AWSSNS='%s'; %s", executable, testSNSARN, tt.overlay)
			input := tt.input
			if input == "" {
				input = testutil.ValidEvent
			}
			var stdout, stderr bytes.Buffer
			assert.Equal(t, tt.code, Run(context.Background(), []string{"send-legacy", "--method", "awssns", "--role", "ops", "--config", writeConfig(t, text)}, strings.NewReader(input), &stdout, &stderr), stderr.String())
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), tt.err)
			if len(tt.want) == 0 {
				assert.NoFileExists(t, capture)
				return
			}
			captures := readCommandCaptures(t, capture)
			require.Len(t, captures, len(tt.want))
			for i, arn := range tt.want {
				assert.Equal(t, expectedSNSCapture(t, captures[i], "imds", strings.Split(arn, ":")[3], arn, "WARNING", "WARNING on test-node at 2026-09-14T12:00:00Z: test.chart 42.5 C", nil), captures[i])
				assertSNSHomeRemoved(t, captures[i])
			}
		})
	}
}

func TestRunLegacySNSPreflight(t *testing.T) {
	for name, tt := range map[string]struct{ overlay, err string }{
		"mode required":                  {"AWSSNS_CREDENTIAL_SOURCE=''", "AWSSNS_CREDENTIAL_SOURCE"},
		"mode invalid":                   {"AWSSNS_CREDENTIAL_SOURCE=synthetic-private-value", "credential_source must be"},
		"static fields missing":          {"AWSSNS_CREDENTIAL_SOURCE=static", "missing a required"},
		"wrong mode credential":          {"AWS_ACCESS_KEY_ID=synthetic-private-value", "outside the selected"},
		"ambient profile setting":        {"AWS_PROFILE=synthetic-private-value", "outside the selected"},
		"endpoint setting":               {"AWS_ENDPOINT_URL=https://synthetic-private-value.invalid", "outside the selected"},
		"web identity role invalid":      {"AWSSNS_CREDENTIAL_SOURCE=web_identity; AWS_ROLE_ARN=synthetic-private-value; AWS_WEB_IDENTITY_TOKEN_FILE=/unread/token", "must be an IAM role"},
		"ecs URI invalid":                {"AWSSNS_CREDENTIAL_SOURCE=ecs; AWS_CONTAINER_CREDENTIALS_RELATIVE_URI='@synthetic-private-value/path'", "relative path"},
		"invalid second target":          {"DEFAULT_RECIPIENT_AWSSNS='" + testSNSARN + " synthetic-private-value'", "target_arn"},
		"relative executable":            {"aws=synthetic-private-value", "absolute path"},
		"configured PATH":                {"PATH=/synthetic-private-value", "setting PATH"},
		"body too long":                  {"AWSSNS_MESSAGE_FORMAT='" + strings.Repeat("x", 262145) + "'", "at most 262144 bytes"},
		"date mutation":                  {"date=synthetic-private-value", "setting date"},
		"value string mutation":          {"value_string=synthetic-private-value", "setting value_string"},
		"previous value string mutation": {"old_value_string=synthetic-private-value", "setting old_value_string"},
		"status message mutation":        {"status_message=synthetic-private-value", "setting status_message"},
	} {
		t.Run(name, func(t *testing.T) {
			dst, capture := snsHelperDestination(t, "")
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(200) }))
			defer server.Close()
			text := fmt.Sprintf("aws='%s'; AWSSNS_CREDENTIAL_SOURCE=imds; DEFAULT_RECIPIENT_AWSSNS='%s'; DISCORD_WEBHOOK_URL='%s'; DEFAULT_RECIPIENT_DISCORD=channel; %s", dst["executable"], testSNSARN, server.URL, tt.overlay)
			var stdout, stderr bytes.Buffer
			assert.Equal(t, 1, Run(context.Background(), []string{"send-legacy", "--method", "discord", "--method", "awssns", "--role", "ops", "--config", writeConfig(t, text)}, strings.NewReader(testutil.ValidEvent), &stdout, &stderr), stderr.String())
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), tt.err)
			assert.Contains(t, stderr.String(), "0 succeeded, 0 failed")
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
			assert.Zero(t, calls.Load())
			assert.NoFileExists(t, capture)
		})
	}
}

func TestRunLegacySNSProducerContextDuplicates(t *testing.T) {
	for name, input := range duplicateProducerContextInputs(t) {
		t.Run(name, func(t *testing.T) {
			dst, capture := snsHelperDestination(t, "")
			config := fmt.Sprintf("aws='%s'; AWSSNS_CREDENTIAL_SOURCE=imds; DEFAULT_RECIPIENT_AWSSNS='%s'; ", dst["executable"], testSNSARN) + `AWSSNS_MESSAGE_FORMAT="$src $unique_id $value_string"`
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), []string{"send-legacy", "--config", writeConfig(t, config), "--method", "awssns", "--role", "ops"}, strings.NewReader(input), &stdout, &stderr)
			assert.Equal(t, 1, code, stderr.String())
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), "invalid JSON event")
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
			assert.NoFileExists(t, capture)
		})
	}
}
