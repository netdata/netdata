// SPDX-License-Identifier: GPL-3.0-or-later

package email

import (
	"io"
	"mime"
	"net/mail"
	"path/filepath"
	"strings"
	"testing"
	"time"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmailConfig(t *testing.T) {
	for name, test := range map[string]struct {
		change func(*Config)
		err    string
	}{
		"defaults": {},
		"display names and sender": {change: func(d *Config) {
			d.Recipients = []string{"Ops Team <ops@example.com>", "root"}
			d.From = "Παρακολούθηση <notify@example.com>"
		}},
		"local sender":          {change: func(d *Config) { d.From = "netdata" }},
		"quoted mailbox":        {change: func(d *Config) { d.From = `"alert bot"@example.com` }},
		"explicit modes":        {change: func(d *Config) { d.PlainTextOnly = new(true); d.Threading = new(false) }},
		"environment reference": {change: func(d *Config) { d.Env = map[string]string{"HOME": "${env:UNREAD_EMAIL_HOME}"} }},
		"no recipients":         {change: func(d *Config) { d.Recipients = nil }, err: "at least one recipient"},
		"relative executable":   {change: func(d *Config) { d.Executable = "synthetic-private-value" }, err: "absolute path"},
		"invalid env":           {change: func(d *Config) { d.Env = map[string]string{"BAD=KEY": "synthetic-private-value"} }, err: "env names"},
	} {
		t.Run(name, func(t *testing.T) {
			dst := Config{Executable: filepath.Join(t.TempDir(), "sendmail"), Recipients: []string{"root"}}
			if test.change != nil {
				test.change(&dst)
			}
			checkConfig(t, dst, test.err)
		})
	}
}

func TestEmailAddresses(t *testing.T) {
	for name, test := range map[string]struct {
		input, envelope string
		invalid         bool
	}{
		"mailbox":                {input: "ops@example.com", envelope: "ops@example.com"},
		"display name":           {input: "Operations <ops@example.com>", envelope: "ops@example.com"},
		"unicode display":        {input: "Ειδοποίηση <ops@example.com>", envelope: "ops@example.com"},
		"international mailbox":  {input: "δοκιμή@example.com", envelope: "δοκιμή@example.com"},
		"quoted local part":      {input: `"ops team"@example.com`, envelope: `"ops team"@example.com`},
		"local root":             {input: "root", envelope: "root"},
		"local plus":             {input: "ops-alerts+test", envelope: "ops-alerts+test"},
		"empty":                  {invalid: true},
		"option":                 {input: "-Xsynthetic-private-value", invalid: true},
		"mailbox option":         {input: "-X@example.com", invalid: true},
		"quoted mailbox option":  {input: `"-X"@example.com`, invalid: true},
		"program":                {input: "|synthetic-private-value", invalid: true},
		"mailbox program":        {input: "|command@example.com", invalid: true},
		"file":                   {input: "/tmp/synthetic-private-value", invalid: true},
		"recipient list":         {input: "ops@example.com, extra@example.com", invalid: true},
		"single mailbox group":   {input: "team: ops@example.com;", envelope: "ops@example.com"},
		"multiple mailbox group": {input: "team: ops@example.com, extra@example.com;", invalid: true},
		"header injection":       {input: "ops@example.com\r\nBcc: extra@example.com", invalid: true},
		"NUL":                    {input: "ops\x00@example.com", invalid: true},
		"encoded control":        {input: "=?UTF-8?B?DQo=?= <ops@example.com>", invalid: true},
		"invalid UTF8":           {input: "\xff@example.com", invalid: true},
		"encoded invalid UTF8":   {input: "=?UTF-8?B?/w==?= <ops@example.com>", invalid: true},
		"reference":              {input: "${env:MAIL_TO}", invalid: true},
		"overlong mailbox":       {input: strings.Repeat("x", 990) + "@example.com", invalid: true},
	} {
		t.Run(name, func(t *testing.T) {
			header, envelope, err := emailAddress(test.input)
			if test.invalid {
				require.Error(t, err)
				assert.Empty(t, header)
				assert.Empty(t, envelope)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
				for field := range map[string]struct{}{"recipient": {}, "from": {}} {
					dst := Config{Executable: filepath.Join(t.TempDir(), "sendmail"), Recipients: []string{"root"}}
					if field == "recipient" {
						dst.Recipients = []string{test.input}
					} else {
						dst.From = test.input
					}
					if field == "from" && test.input == "" {
						continue
					}
					checkConfig(t, dst, "expected a literal mailbox")
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.envelope, envelope)
			if strings.Contains(test.envelope, "@") {
				want, err := mail.ParseAddress(test.input)
				require.NoError(t, err)
				got, err := mail.ParseAddress(strings.ReplaceAll(header, "\r\n", ""))
				require.NoError(t, err)
				assert.Equal(t, want, got)
			} else {
				assert.Equal(t, test.input, header)
			}
		})
	}
}

func TestEmailModeYAML(t *testing.T) {
	for field := range map[string]struct{}{"plain_text_only": {}, "threading": {}} {
		for name, test := range map[string]struct {
			value string
			want  *bool
			bad   bool
		}{
			"default": {}, "true": {value: "true", want: new(true)}, "false": {value: "false", want: new(false)},
			"wrong type": {value: "123", bad: true}, "string": {value: "synthetic-private-value", bad: true},
		} {
			t.Run(field+"/"+name, func(t *testing.T) {
				raw := "version: 1\ndestinations:\n  mail:\n    type: email\n    executable: " + filepath.Join(t.TempDir(), "sendmail") + "\n    recipients: [root]\n"
				if test.value != "" {
					raw += "    " + field + ": " + test.value + "\n"
				}
				cfg, err := readConfig(strings.NewReader(raw))
				if test.bad {
					require.ErrorContains(t, err, "invalid YAML")
					assert.NotContains(t, err.Error(), "synthetic-private-value")
					return
				}
				require.NoError(t, err)
				want := Config{Executable: cfg.Destinations["mail"].Executable, Recipients: []string{"root"}}
				if field == "threading" {
					want.Threading = test.want
				} else {
					want.PlainTextOnly = test.want
				}
				assert.Equal(t, want, cfg.Destinations["mail"])
			})
		}
	}
}

func TestRenderEmail(t *testing.T) {
	for name, test := range map[string]struct {
		status, wording string
		plain           bool
		threading       *bool
		from            string
	}{
		"default warning":          {status: "WARNING", wording: "needs attention"},
		"critical with sender":     {status: "CRITICAL", wording: "is critical", from: "Παρακολούθηση <notify@example.com>"},
		"plain recovery no thread": {status: "CLEAR", wording: "recovered", plain: true, threading: new(false), from: "netdata"},
	} {
		t.Run(name, func(t *testing.T) {
			dst := Config{Recipients: []string{"Ops Team <ops@example.com>", "root"}, From: test.from, PlainTextOnly: new(test.plain), Threading: test.threading}
			event := testutil.ExpectedEvent()
			event.Status = test.status
			event.Duration, event.NonClearDuration = new(uint32(0)), new(uint32(123))
			event.URL = "https://example.com/alert?a=1&b=2"
			before := time.Now().UTC().Truncate(time.Second)
			args, raw, err := renderEmail(dst, event)
			require.NoError(t, err)
			header, parts := testutil.ParseEmail(t, raw)
			date, err := header.Date()
			require.NoError(t, err)
			assert.WithinRange(t, date, before, time.Now().UTC())
			assert.Regexp(t, `^<[A-Z2-7]{26}@netdata\.invalid>$`, header.Get("Message-ID"))
			subject, err := new(mime.WordDecoder).DecodeHeader(header.Get("Subject"))
			require.NoError(t, err)
			assert.Equal(t, "test-node "+test.wording+": test alert (test.chart)", subject)
			wantHeaders := mail.Header{
				"To":      {"=?UTF-8?B?T3BzIFRlYW0=?= <ops@example.com>, root"},
				"Subject": {header.Get("Subject")}, "Date": {header.Get("Date")}, "Message-Id": {header.Get("Message-ID")},
				"Mime-Version": {"1.0"}, "Content-Type": {header.Get("Content-Type")},
				"X-Netdata-Severity": {strings.ToLower(test.status)}, "X-Netdata-Alert-Name": {"=?UTF-8?B?dGVzdF9hbGVydA==?="},
				"X-Netdata-Chart": {"=?UTF-8?B?dGVzdC5jaGFydA==?="}, "X-Netdata-Host": {"=?UTF-8?B?dGVzdC1ub2Rl?="},
			}
			wantArgs := []string{"-t", "-i"}
			if test.from != "" {
				if test.from == "netdata" {
					wantHeaders["From"] = []string{"netdata"}
					wantArgs = append(wantArgs, "-f", "netdata")
				} else {
					wantArgs = append(wantArgs, "-f", "notify@example.com")
					got, err := header.AddressList("From")
					require.NoError(t, err)
					assert.Equal(t, []*mail.Address{{Name: "Παρακολούθηση", Address: "notify@example.com"}}, got)
					wantHeaders["From"] = []string{header.Get("From")}
				}
			}
			if test.threading == nil || *test.threading {
				thread := "<netdata.19733ce528b55ea98c45cf2aeb1d4af8d9f2ba9cf546832724e21baaa8ae958b@netdata.invalid>"
				wantHeaders["In-Reply-To"], wantHeaders["References"] = []string{thread}, []string{thread}
			}
			status := "CLEAR → " + test.status
			if test.status == "CLEAR" {
				status = "CLEAR"
			}
			plain := "Temperature is high\r\nA quote: \"hot\"\r\nUnicode: θερμοκρασία\r\nNode: test-node\r\nAlert: test_alert\r\nStatus: " + status + "\r\nChart: test.chart\r\nContext: test.context\r\nValue: 42.5 C\r\nPrevious value: 0 C\r\nTime: 2026-09-14T12:00:00Z\r\nhttps://example.com/alert?a=1&b=2\r\nIncident ID: test-incident\r\nDuration: 0 seconds\r\nNon-clear duration: 123 seconds\r\n"
			wantParts := []testutil.EmailPart{{Kind: "text/plain", Body: plain}}
			if test.plain {
				wantHeaders["Content-Transfer-Encoding"] = []string{"quoted-printable"}
				assert.Equal(t, "text/plain; charset=UTF-8", header.Get("Content-Type"))
			} else {
				htmlPlain := strings.ReplaceAll(strings.ReplaceAll(plain, "\"", "&#34;"), "&b=2", "&amp;b=2")
				wantParts = append(wantParts, testutil.EmailPart{Kind: "text/html", Body: "<!DOCTYPE html><html><body><pre>" + strings.TrimSuffix(htmlPlain, "\r\n") + "</pre><p><a href=\"https://example.com/alert?a=1&amp;b=2\">View in Netdata</a></p></body></html>\r\n"})
			}
			assert.Equal(t, wantHeaders, header)
			assert.Equal(t, wantArgs, args)
			assert.Equal(t, wantParts, parts)
		})
	}
}

func TestEmailEscapingAndLongLines(t *testing.T) {
	for name, text := range map[string]string{
		"HTML and MIME delimiters": "<script>bad()</script> & \"quote\"\n.\n--boundary\nAfter the dot",
		"header injection":         "node\r\nBcc: extra@example.com",
		"NUL":                      "before\x00after",
		"long ASCII":               strings.Repeat("x", 3000),
		"long Unicode":             strings.Repeat("温度🙂", 700),
	} {
		t.Run(name, func(t *testing.T) {
			event := testutil.ExpectedEvent()
			event.Node = text
			event.Info = text
			dst := Config{Recipients: []string{"ops@example.com"}}
			_, raw, err := renderEmail(dst, event)
			require.NoError(t, err)
			header, parts := testutil.ParseEmail(t, raw)
			subject, err := new(mime.WordDecoder).DecodeHeader(header.Get("Subject"))
			require.NoError(t, err)
			assert.Equal(t, text+" needs attention: test alert (test.chart)", subject)
			assert.Empty(t, header.Get("Bcc"))
			require.Len(t, parts, 2)
			normalized := strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", "\r\n")
			assert.Contains(t, parts[0].Body, normalized)
			assert.NotContains(t, parts[1].Body, "<script>")
			assert.NotContains(t, parts[0].Body, "Duration:")
		})
	}
	for name, display := range map[string]string{"ASCII": strings.Repeat("a", 3000), "Unicode": strings.Repeat("名", 1000)} {
		t.Run("display name/"+name, func(t *testing.T) {
			dst := Config{Recipients: []string{display + " <ops@example.com>"}, From: display + " <notify@example.com>"}
			_, raw, err := renderEmail(dst, testutil.ExpectedEvent())
			require.NoError(t, err)
			header, _ := testutil.ParseEmail(t, raw)
			for field, address := range map[string]string{"To": "ops@example.com", "From": "notify@example.com"} {
				got, err := header.AddressList(field)
				require.NoError(t, err)
				assert.Equal(t, []*mail.Address{{Name: display, Address: address}}, got)
			}
		})
	}
}

func TestEmailThreadIdentity(t *testing.T) {
	base := testutil.ExpectedEvent()
	dst := Config{Recipients: []string{"root"}}
	_, raw, err := renderEmail(dst, base)
	require.NoError(t, err)
	header, _ := testutil.ParseEmail(t, raw)
	for name, test := range map[string]struct {
		change func(*notifyevent.Event)
		same   bool
	}{
		"repeat":                  {same: true},
		"new incident and status": {change: func(e *notifyevent.Event) { e.IncidentID = "other"; e.Status = "CLEAR" }, same: true},
		"node":                    {change: func(e *notifyevent.Event) { e.Node += "other" }},
		"chart":                   {change: func(e *notifyevent.Event) { e.Chart = "" }},
		"alert":                   {change: func(e *notifyevent.Event) { e.Alert += "other" }},
	} {
		t.Run(name, func(t *testing.T) {
			event := base
			if test.change != nil {
				test.change(&event)
			}
			_, raw, err := renderEmail(dst, event)
			require.NoError(t, err)
			got, _ := testutil.ParseEmail(t, raw)
			assert.NotEqual(t, header.Get("Message-ID"), got.Get("Message-ID"))
			assert.Equal(t, test.same, header.Get("References") == got.Get("References"))
		})
	}
	base.Node, base.Chart, base.Alert = "a-b", "c", "d"
	_, raw, err = renderEmail(dst, base)
	require.NoError(t, err)
	one, _ := testutil.ParseEmail(t, raw)
	base.Node, base.Chart = "a", "b-c"
	_, raw, err = renderEmail(dst, base)
	require.NoError(t, err)
	two, _ := testutil.ParseEmail(t, raw)
	assert.NotEqual(t, one.Get("References"), two.Get("References"))
}

func TestEmailOptionalContent(t *testing.T) {
	for name, test := range map[string]struct {
		change func(*notifyevent.Event)
		extra  string
	}{
		"omitted":                    {},
		"zero duration":              {change: func(e *notifyevent.Event) { e.Duration = new(uint32(0)) }, extra: "Duration: 0 seconds\r\n"},
		"maximum non-clear duration": {change: func(e *notifyevent.Event) { e.NonClearDuration = new(uint32(4294967295)) }, extra: "Non-clear duration: 4294967295 seconds\r\n"},
	} {
		t.Run(name, func(t *testing.T) {
			event := notifyevent.Event{Version: 1, IncidentID: "id", Timestamp: time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC), Node: "node", Alert: "alert", Status: "CLEAR", Summary: "Recovered"}
			if test.change != nil {
				test.change(&event)
			}
			_, raw, err := renderEmail(Config{Recipients: []string{"root"}, PlainTextOnly: new(true)}, event)
			require.NoError(t, err)
			header, parts := testutil.ParseEmail(t, raw)
			assert.Empty(t, header.Get("X-Netdata-Chart"))
			subject, err := new(mime.WordDecoder).DecodeHeader(header.Get("Subject"))
			require.NoError(t, err)
			assert.Equal(t, "node recovered: alert", subject)
			assert.Equal(t, []testutil.EmailPart{{Kind: "text/plain", Body: "Recovered\r\nNode: node\r\nAlert: alert\r\nStatus: CLEAR\r\nTime: 2026-09-16T12:00:00Z\r\nIncident ID: id\r\n" + test.extra}}, parts)
		})
	}
}

func readConfig(r io.Reader) (testutil.Document[Config], error) {
	return testutil.ReadConfig(r, "email", func(c Config) error { _, err := New(c, nil); return err })
}

func checkConfig(t *testing.T, c Config, want string) {
	t.Helper()
	_, err := New(c, nil)
	if want != "" {
		require.ErrorContains(t, err, want)
		assert.NotContains(t, err.Error(), "synthetic-private-value")
	} else {
		require.NoError(t, err)
	}
}

func TestConstructorOwnsConfiguration(t *testing.T) {
	original := func() Config {
		return Config{Executable: "/usr/bin/helper", Recipients: []string{"root"}, Threading: new(true), PlainTextOnly: new(false), Env: map[string]string{"TOKEN": "literal"}}
	}
	for name, mutate := range map[string]func(*Config){
		"environment": func(c *Config) { c.Env["TOKEN"] = "changed" },
		"recipients":  func(c *Config) { c.Recipients[0] = "other" },
		"threading":   func(c *Config) { *c.Threading = false },
		"plain text":  func(c *Config) { *c.PlainTextOnly = true },
	} {
		t.Run(name, func(t *testing.T) {
			config := original()
			sender, err := New(config, nil)
			require.NoError(t, err)
			mutate(&config)
			assert.Equal(t, original(), sender.config)
		})
	}
}
