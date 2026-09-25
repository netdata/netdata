// SPDX-License-Identifier: GPL-3.0-or-later

package app

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunCheckLegacy(t *testing.T) {
	tests := map[string]struct {
		files []string
		args  []string
		code  int
		err   string
	}{
		"ordinary settings":          {files: []string{`SEND_EMAIL=YES; DEFAULT_RECIPIENT_EMAIL='ops@example.com'`}},
		"overlay syntax":             {files: []string{`ROLE=sysadmin`, `role_recipients_email[sysadmin]='ops@example.com'`}},
		"unset event reference":      {files: []string{`MESSAGE="$host $status"`}},
		"unresolved literal secret":  {files: []string{`TOKEN='${file:/synthetic-private-value/missing}'`}},
		"inert function":             {files: []string{`custom_sender() { curl "$(secret-tool read)"; }`}},
		"top-level command":          {files: []string{`echo synthetic-private-value`}, code: 1, err: "file 1: line 1, column 1: top-level commands"},
		"invalid overlay":            {files: []string{`A=ok`, "# comment\nA='synthetic-private-value"}, code: 1, err: "file 2: line 2, column"},
		"reject after valid overlay": {files: []string{`A=ok`, `B=ok`, `source synthetic-private-value`}, code: 1, err: "file 3:"},
		"no role selector":           {files: []string{`A=ok`}, args: []string{"--role", "sysadmin"}, code: 1, err: "invalid command options"},
		"no destination selector":    {files: []string{`A=ok`}, args: []string{"--destination", "dev"}, code: 1, err: "invalid command options"},
		"no config":                  {code: 1, err: "provide --config"},
		"timeout required positive":  {files: []string{`A=ok`}, args: []string{"--timeout", "0"}, code: 1, err: "positive --timeout"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			args := append([]string{"check-legacy"}, test.args...)
			for _, contents := range test.files {
				args = append(args, "--config", writeConfig(t, contents))
			}
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), args, panicReader{}, &stdout, &stderr)
			assert.Equal(t, test.code, code)
			if test.code == 0 {
				assert.Equal(t, "legacy configuration syntax is supported; delivery is not validated\n", stdout.String())
				assert.Empty(t, stderr.String())
			} else {
				assert.Empty(t, stdout.String())
				assert.Contains(t, stderr.String(), test.err)
			}
			assert.NotContains(t, stderr.String()+stdout.String(), "synthetic-private-value")
		})
	}
}

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("check-legacy must not read event input") }

func TestRunCheckLegacyStockWithoutBash(t *testing.T) {
	// A usable shell cannot be discovered, even if a future regression attempts to run one.
	t.Setenv("PATH", t.TempDir())
	marker := filepath.ToSlash(filepath.Join(t.TempDir(), "executed"))
	t.Setenv("BASH_ENV", marker)
	stock := filepath.Join("..", "..", "..", "health_alarm_notify.conf")
	overlay := writeConfig(t, fmt.Sprintf("SEND_CUSTOM=YES\ncustom_sender() { printf executed >'%s'; }", marker))
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"check-legacy", "--config", stock, "--config", overlay}, panicReader{}, &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Equal(t, "legacy configuration syntax is supported; delivery is not validated\n", stdout.String())
	assert.Empty(t, stderr.String())
	assert.NoFileExists(t, marker)
}

func TestRunCheckLegacyControl(t *testing.T) {
	tests := map[string]struct {
		args      []string
		cancel    bool
		wantCode  int
		wantError string
	}{
		"help":         {args: []string{"check-legacy", "--help"}},
		"missing file": {args: []string{"check-legacy", "--config", filepath.Join(t.TempDir(), "synthetic-private-value")}, wantCode: 1, wantError: "file 1: could not open file"},
		"canceled":     {args: []string{"check-legacy", "--config", writeConfig(t, "A=ok")}, cancel: true, wantCode: 1, wantError: "canceled"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if test.cancel {
				cancel()
			}
			var stdout, stderr bytes.Buffer
			assert.Equal(t, test.wantCode, Run(ctx, test.args, strings.NewReader(""), &stdout, &stderr))
			if test.wantCode == 0 {
				assert.Equal(t, usage, stdout.String())
			} else {
				assert.Empty(t, stdout.String())
			}
			assert.Contains(t, stderr.String(), test.wantError)
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
		})
	}
}
