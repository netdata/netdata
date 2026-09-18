// SPDX-License-Identifier: GPL-3.0-or-later

package legacycustom

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/commandexec"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/legacyconfig"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"
)

func runtimeSettings(t *testing.T, body string) legacyconfig.Settings {
	t.Helper()
	return legacyconfig.Settings{Variables: map[string]string{"bash": testutil.Bash(t)}, Functions: map[string]string{"custom_sender": "custom_sender() {\n" + body + "\n}"}}
}

func TestRuntimeLiteralState(t *testing.T) {
	bash := testutil.Bash(t)
	for name, value := range map[string]string{
		"quotes": "'\"\\\ntrailing\n", "commands": "$(printf injected); `printf injected`; ${env:SECRET}",
		"unicode": "温度 θερμοκρασία", "empty": "", "arithmetic": "x[$(printf injected)]", "options": "-n -e -- * ? [x]",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			capture := filepath.Join(dir, "capture")
			marker := filepath.Join(dir, "startup")
			startup := filepath.Join(dir, "bash_env")
			require.NoError(t, os.WriteFile(startup, []byte("printf executed >"+quote(marker)), 0600))
			t.Setenv("BASH_ENV", startup)
			t.Setenv("ENV", startup)
			t.Setenv("CUSTOM_AMBIENT", "must-not-leak")
			settings := legacyconfig.Settings{
				Variables:  map[string]string{"bash": bash, "capture": capture, "literal": value},
				Recipients: map[string]map[string]string{"role_recipients_custom": {value + "key": value}},
				Functions: map[string]string{"custom_sender": `custom_sender() { helper "$1"; }`, "helper": `helper() {
 printf '%s\000' "$literal" "${role_recipients_custom[@]}" "$1" "$to_custom" "${CUSTOM_AMBIENT-unset}" "${BASH_ENV-unset}" "$HOME" >"$capture"
 cat >/dev/null
}`},
			}
			runner := &commandexec.Runner{}
			defer runner.CloseAndWait()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			sender, err := New(ctx, settings, []string{"one", "two"}, testutil.ExpectedEvent(), runner)
			require.NoError(t, err)
			assert.NoFileExists(t, capture)
			assert.NoFileExists(t, marker)
			// Preparation owns its snapshot, including retained functions.
			settings.Variables["literal"] = "changed"
			settings.Functions["helper"] = "changed"
			require.NoError(t, sender.Send(ctx, testutil.ExpectedEvent()))
			data, err := os.ReadFile(capture)
			require.NoError(t, err)
			got := strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
			require.Len(t, got, 7)
			assert.Equal(t, []string{value, value, "one two", "one two", "unset", "unset"}, got[:6])
			require.NotEmpty(t, got[6])
			assert.NoDirExists(t, got[6])
			assert.NoFileExists(t, marker)
		})
	}
}

func TestRuntimeHelpers(t *testing.T) {
	for name, tt := range map[string]struct{ call, want string }{
		"url bytes":     {`urlencode 'a b/温度'`, "a%20b%2f%e6%b8%a9%e5%ba%a6"},
		"url option":    {`urlencode '-n'`, "-n"},
		"zero seconds":  {`duration4human 0`, "0 second"},
		"one second":    {`duration4human 1`, "1 second"},
		"minutes":       {`duration4human 61`, "1 minute and 1 second"},
		"rounded hours": {`duration4human 3630`, "1 hour and 1 minute"},
		"rounded days":  {`duration4human 88200`, "1 day and 1 hour"},
		"leading zero":  {`duration4human 08`, "8 seconds"},
		"maximum":       {`duration4human 4294967295`, "49710 days and 6 hours"},
	} {
		t.Run(name, func(t *testing.T) {
			capture := filepath.Join(t.TempDir(), "capture")
			settings := runtimeSettings(t, tt.call+` >"$capture"; printf '%s\n' "$REPLY" >>"$capture"`)
			settings.Variables["capture"] = capture
			runner := &commandexec.Runner{}
			defer runner.CloseAndWait()
			sender, err := New(t.Context(), settings, []string{"one"}, testutil.ExpectedEvent(), runner)
			require.NoError(t, err)
			require.NoError(t, sender.Send(t.Context(), testutil.ExpectedEvent()))
			data, err := os.ReadFile(capture)
			require.NoError(t, err)
			assert.Equal(t, tt.want+"\n"+tt.want+"\n", string(data))
		})
	}
}

func TestRuntimeFailures(t *testing.T) {
	bash := testutil.Bash(t)
	for name, tt := range map[string]struct {
		values                    map[string]string
		function, executable, err string
		send                      bool
	}{
		"missing":                       {function: "", err: "function is required"},
		"stock default":                 {function: stockDefault, err: "stock placeholder"},
		"stock example":                 {function: stockExample, err: "stock placeholder"},
		"function source extra command": {function: "custom_sender() { :; }; printf injected", err: "one plain named"},
		"function source NUL":           {function: "custom_sender() { :; }\x00", err: "one plain named"},
		"scalar NUL":                    {values: map[string]string{"extra": "x\x00y"}, err: "NUL"},
		"startup file":                  {values: map[string]string{"BASH_ENV": "synthetic-private-value"}, err: "reserved shell variable"},
		"integer shell variable":        {values: map[string]string{"SECONDS": "x[$(printf injected)]"}, err: "reserved shell variable"},
		"PATH":                          {values: map[string]string{"PATH": "/synthetic-private-value"}, err: "reserved shell variable"},
		"internal array":                {values: map[string]string{"_netdata_custom_curl_options": "x"}, err: "reserved shell variable"},
		"missing executable":            {executable: "/does-not-exist/synthetic-private-value", err: "could not start command"},
		"relative executable":           {executable: "./bash", err: "absolute path"},
		"wrong executable":              {executable: "/usr/bin/true", err: "Bash"},
		"nonzero":                       {function: `custom_sender() { printf 'synthetic-private-value' >&2; return 23; }`, send: true, err: "status 23"},
		"bad duration expression":       {function: `custom_sender() { duration4human '1 + 1'; }`, send: true, err: "status 1"},
		"bad duration negative":         {function: `custom_sender() { duration4human -1; }`, send: true, err: "status 1"},
		"bad duration overflow":         {function: `custom_sender() { duration4human 4294967296; }`, send: true, err: "status 1"},
	} {
		t.Run(name, func(t *testing.T) {
			settings := legacyconfig.Settings{Variables: map[string]string{"bash": bash}, Functions: map[string]string{"custom_sender": `custom_sender() { :; }`}}
			for k, v := range tt.values {
				settings.Variables[k] = v
			}
			if name == "missing" || tt.function != "" {
				settings.Functions["custom_sender"] = tt.function
			}
			if tt.executable != "" {
				settings.Variables["bash"] = tt.executable
			}
			runner := &commandexec.Runner{}
			defer runner.CloseAndWait()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			sender, err := New(ctx, settings, []string{"one"}, testutil.ExpectedEvent(), runner)
			if tt.send {
				require.NoError(t, err)
				err = sender.Send(ctx, testutil.ExpectedEvent())
			}
			require.ErrorContains(t, err, tt.err)
			assert.NotContains(t, err.Error(), "synthetic-private-value")
		})
	}
}

func TestShippedPlaceholders(t *testing.T) {
	for name, path := range map[string]string{"script": "../../../alarm-notify.sh.in", "config": "../../../health_alarm_notify.conf"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			f, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(string(data)), "")
			require.NoError(t, err)
			found := false
			for _, stmt := range f.Stmts {
				fn, ok := stmt.Cmd.(*syntax.FuncDecl)
				if !ok || fn.Name.Value != "custom_sender" {
					continue
				}
				body, err := functionBody("custom_sender", string(data[fn.Pos().Offset():fn.End().Offset()]))
				require.NoError(t, err)
				assert.True(t, stockPlaceholder(body))
				found = true
			}
			require.True(t, found)
		})
	}
	body, err := functionBody("custom_sender", strings.Replace(stockDefault, "\n}", "\n curl https://example.invalid\n}", 1))
	require.NoError(t, err)
	assert.False(t, stockPlaceholder(body))
}

func TestDocurl(t *testing.T) {
	curl, err := exec.LookPath("curl")
	if err != nil {
		t.Skip("curl is not installed")
	}
	for name, tt := range map[string]struct {
		status   int
		expected string
		fail     bool
	}{
		"success": {status: 202, expected: "202"}, "HTTP failure is caller decision": {status: 500, expected: "500"}, "transport exit": {status: 500, fail: true},
	} {
		t.Run(name, func(t *testing.T) {
			calls := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.NoError(t, r.ParseForm())
				calls <- r.Form.Get("Body")
				w.WriteHeader(tt.status)
			}))
			defer server.Close()
			capture := filepath.Join(t.TempDir(), "capture")
			settings := runtimeSettings(t, `docurl --data-urlencode "Body=$text" "$endpoint" >"$capture"`)
			settings.Variables["curl"] = curl
			settings.Variables["capture"] = capture
			settings.Variables["endpoint"] = server.URL
			settings.Variables["text"] = "quotes ' $(literal) 温度"
			settings.Variables["curl_options"] = "--connect-timeout 2"
			if tt.fail {
				settings.Variables["curl_options"] += " --fail"
			}
			runner := &commandexec.Runner{}
			defer runner.CloseAndWait()
			sender, err := New(t.Context(), settings, []string{"one"}, testutil.ExpectedEvent(), runner)
			require.NoError(t, err)
			err = sender.Send(t.Context(), testutil.ExpectedEvent())
			if tt.fail {
				require.ErrorContains(t, err, "status 22")
			} else {
				require.NoError(t, err)
				data, err := os.ReadFile(capture)
				require.NoError(t, err)
				assert.Equal(t, tt.expected, string(data))
			}
			require.Len(t, calls, 1)
			assert.Equal(t, settings.Variables["text"], <-calls)
		})
	}
}

func TestRuntimeCancellation(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "started")
	settings := runtimeSettings(t, `printf '%s' "$HOME" >"$marker"; sleep 30 & wait`)
	settings.Variables["marker"] = marker
	runner := &commandexec.Runner{}
	sender, err := New(t.Context(), settings, []string{"one"}, testutil.ExpectedEvent(), runner)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- sender.Send(ctx, testutil.ExpectedEvent()) }()
	require.Eventually(t, func() bool { _, err := os.Stat(marker); return err == nil }, 5*time.Second, 10*time.Millisecond)
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(3 * time.Second):
		t.Fatal("custom cancellation did not finish")
	}
	runner.CloseAndWait()
	home, err := os.ReadFile(marker)
	require.NoError(t, err)
	assert.NoDirExists(t, string(home))
}

func TestDocurlLiteralOptions(t *testing.T) {
	dir := t.TempDir()
	capture := filepath.Join(dir, "capture")
	fake := filepath.Join(dir, "curl")
	require.NoError(t, os.WriteFile(fake, []byte(fmt.Sprintf("#!/bin/sh\nprintf '%%s\\000' \"$@\" >%s\nexit 17\n", quote(capture))), 0700))
	settings := runtimeSettings(t, `docurl --data "$literal" https://example.invalid`)
	settings.Variables["curl"] = fake
	settings.Variables["curl_options"] = "--header 'not-quoted' * $(literal)"
	settings.Variables["literal"] = "$(never) x y"
	runner := &commandexec.Runner{}
	defer runner.CloseAndWait()
	sender, err := New(t.Context(), settings, []string{"one"}, testutil.ExpectedEvent(), runner)
	require.NoError(t, err)
	require.ErrorContains(t, sender.Send(t.Context(), testutil.ExpectedEvent()), "status 17")
	data, err := os.ReadFile(capture)
	require.NoError(t, err)
	assert.Equal(t, []string{"--header", "'not-quoted'", "*", "$(literal)", "--write-out", "%{http_code}", "--output", "/dev/null", "--silent", "--show-error", "--data", "$(never) x y", "https://example.invalid", ""}, strings.Split(string(data), "\x00"))
}

func TestRuntimeExecutableDiscovery(t *testing.T) {
	bash := testutil.Bash(t)
	dir := t.TempDir()
	require.NoError(t, os.Symlink(bash, filepath.Join(dir, "bash")))
	t.Setenv("PATH", dir)
	settings := runtimeSettings(t, ":")
	delete(settings.Variables, "bash")
	runner := &commandexec.Runner{}
	defer runner.CloseAndWait()
	sender, err := New(t.Context(), settings, []string{"one"}, testutil.ExpectedEvent(), runner)
	require.NoError(t, err)
	require.NoError(t, sender.Send(t.Context(), testutil.ExpectedEvent()))
}
