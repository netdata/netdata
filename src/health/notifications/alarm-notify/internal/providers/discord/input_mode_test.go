// SPDX-License-Identifier: GPL-3.0-or-later

package discord

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInputModeConstructorAndSend(t *testing.T) {
	for name, test := range map[string]struct {
		mode      secret.InputMode
		path      string
		reference string
		invalid   bool
	}{
		"literal env path":              {mode: secret.LiteralInput, path: "/${env:NOTIFIER_DISCORD_LITERAL}"},
		"literal file path":             {mode: secret.LiteralInput, path: "/${file:/notifier/no-such-file}"},
		"native interpolation rejected": {path: "/${env:NOTIFIER_DISCORD_LITERAL}", invalid: true},
		"native env resolved":           {path: "/native-env", reference: "env"},
		"native file resolved":          {path: "/native-file", reference: "file"},
	} {
		t.Run(name, func(t *testing.T) {
			var paths, queries []string
			client := &http.Client{Transport: inputModeTransport(func(r *http.Request) (*http.Response, error) {
				paths = append(paths, r.URL.Path)
				queries = append(queries, r.URL.RawQuery)
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
			})}
			endpoint := "https://example.com" + test.path
			switch test.reference {
			case "env":
				t.Setenv("NOTIFIER_DISCORD_ENDPOINT", endpoint)
				endpoint = "${env:NOTIFIER_DISCORD_ENDPOINT}"
			case "file":
				file := filepath.Join(t.TempDir(), "endpoint")
				require.NoError(t, os.WriteFile(file, []byte(endpoint), 0600))
				endpoint = "${file:" + file + "}"
			}
			sender, err := New(Config{Secrets: test.mode, URL: endpoint}, client)
			if test.invalid {
				require.Error(t, err)
				assert.Nil(t, sender)
				assert.Empty(t, paths)
				return
			}
			require.NoError(t, err)
			require.NoError(t, sender.Send(t.Context(), testutil.ExpectedEvent()))
			assert.Equal(t, []string{test.path}, paths)
			assert.Equal(t, []string{"wait=true"}, queries)
		})
	}
}

type inputModeTransport func(*http.Request) (*http.Response, error)

func (f inputModeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
