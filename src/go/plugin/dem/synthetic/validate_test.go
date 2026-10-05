// SPDX-License-Identifier: GPL-3.0-or-later
package synthetic

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidateRequest(t *testing.T) {
	for name, tc := range map[string]struct {
		change func(*Request)
		valid  bool
	}{
		"inline":             {func(*Request) {}, true},
		"whitespace":         {func(r *Request) { r.Script = " \t\n" }, false},
		"unicode whitespace": {func(r *Request) { r.Script = "\u0085\u00a0\u2003" }, false},
		"no source":          {func(r *Request) { r.Script = "" }, false},
		"both sources":       {func(r *Request) { r.ScriptPath = "/nonexistent/entry.ts" }, false},
		"absolute ts syntax": {func(r *Request) { r.Script = ""; r.ScriptPath = "/nonexistent/entry.ts" }, true},
		"relative path":      {func(r *Request) { r.Script = ""; r.ScriptPath = "entry.ts" }, false},
		"wrong extension":    {func(r *Request) { r.Script = ""; r.ScriptPath = "/entry.txt" }, false},
		"blank name":         {func(r *Request) { r.Name = " " }, false},
		"short timeout":      {func(r *Request) { r.Timeout = time.Millisecond - 1 }, false},
		"minimum timeout":    {func(r *Request) { r.Timeout = time.Millisecond }, true},
		"unsupported kind":   {func(r *Request) { r.Kind = "other" }, false},
		"secret":             {func(r *Request) { r.Secrets = map[string]string{"DEM_SECRET_TOKEN": ""} }, true},
		"secret name":        {func(r *Request) { r.Secrets = map[string]string{"PATH": "x"} }, false},
		"secret NUL":         {func(r *Request) { r.Secrets = map[string]string{"DEM_SECRET_TOKEN": "a\x00b"} }, false},
		"lighthouse":         {func(r *Request) { r.Kind = Lighthouse; r.URL = "https://example.org/" }, true},
		"uppercase scheme":   {func(r *Request) { r.Kind = Lighthouse; r.URL = "HTTPS://example.org/" }, true},
		"relative URL":       {func(r *Request) { r.Kind = Lighthouse; r.URL = "/home" }, false},
		"credential URL":     {func(r *Request) { r.Kind = Lighthouse; r.URL = "https://user:pass@example.org/" }, false},
	} {
		t.Run(name, func(t *testing.T) {
			r := Request{
				Kind:    Journey,
				Name:    "check",
				Script:  "export {};",
				Timeout: time.Minute,
			}
			tc.change(&r)
			err := ValidateRequest(r)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestBlankScriptDiagnostic(t *testing.T) {
	require.EqualError(t, ValidateRequest(Request{
		Kind: Journey, Name: "check", Script: " \t\n\u2003", Timeout: time.Minute,
	}), "script must not be blank")
}
