// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validConfig = "version: 1\ndestinations:\n  dev:\n    type: webhook\n    url: https://example.com/notify\n"

func TestReadConfig(t *testing.T) {
	base := Config{
		Version: 1,
		Destinations: map[string]Destination{
			"dev": {Type: "webhook", URL: "https://example.com/notify"},
		},
	}
	routed := base
	routed.Routing = Routing{
		Default: []string{"dev"},
		Roles:   map[string][]string{"sysadmin": {"dev"}, "muted": {}},
	}
	tests := map[string]struct {
		input string
		want  Config
		err   string
	}{
		"valid": {input: validConfig, want: base},
		"routing": {
			input: validConfig + "routing:\n  default: [dev]\n  roles:\n    sysadmin: [dev]\n    muted: []\n",
			want:  routed,
		},
		"unknown routing field": {
			input: validConfig + "routing:\n  fallback: [dev]\n",
			err:   "invalid YAML",
		},
		"dangling default": {
			input: validConfig + "routing:\n  default: [synthetic-private-value]\n",
			err:   "unconfigured destination",
		},
		"dangling role destination": {
			input: validConfig + "routing:\n  roles:\n    sysadmin: [synthetic-private-value]\n",
			err:   "unconfigured destination",
		},
		"empty role name": {
			input: validConfig + "routing:\n  roles:\n    ' ': [dev]\n",
			err:   "role name must not be empty",
		},
		"silent role configured": {
			input: validConfig + "routing:\n  roles:\n    silent: [dev]\n",
			err:   "reserved roles",
		},
		"disabled role configured": {
			input: validConfig + "routing:\n  roles:\n    disabled: []\n",
			err:   "reserved roles",
		},
		"null role destinations": {
			input: validConfig + "routing:\n  roles:\n    sysadmin:\n",
			err:   "use [] to suppress",
		},
		"scalar role destinations": {
			input: validConfig + "routing:\n  roles:\n    sysadmin: dev\n",
			err:   "invalid YAML",
		},
		"duplicate role": {
			input: validConfig + "routing:\n  roles:\n    sysadmin: [dev]\n    sysadmin: []\n",
			err:   "invalid YAML",
		},
		"trailing comment": {input: validConfig + "# done\n", want: base},
		"unknown field": {
			input: validConfig + "    unknown_setting: synthetic-private-value\n",
			err:   "invalid YAML",
		},
		"wrong scalar type": {
			input: strings.Replace(
				validConfig,
				"version: 1",
				"version: synthetic-private-value",
				1,
			),
			err: "invalid YAML",
		},
		"duplicate key": {
			input: validConfig + "    url: https://example.com/other\n",
			err:   "invalid YAML",
		},
		"empty":          {err: "invalid YAML"},
		"invalid syntax": {input: "version: [", err: "invalid YAML"},
		"null":           {input: "null", err: "version must be 1"},
		"wrong version": {
			input: strings.Replace(validConfig, "version: 1", "version: 2", 1),
			err:   "version must be 1",
		},
		"no destinations": {input: "version: 1", err: "at least one destination"},
		"blank destination": {
			input: strings.Replace(validConfig, "dev:", "' ':", 1),
			err:   "name must not be empty",
		},
		"unsupported provider": {
			input: strings.Replace(validConfig, "webhook", "unimplemented", 1),
			err:   "other providers are not implemented",
		},
		"second document": {
			input: validConfig + "---\n" + validConfig,
			err:   "exactly one YAML",
		},
		"second empty document": {input: validConfig + "---\n", err: "exactly one YAML"},
		"relative URL": {
			input: strings.Replace(validConfig, "https://example.com/notify", "/notify", 1),
			err:   "absolute HTTP(S)",
		},
		"URL credentials": {
			input: strings.Replace(
				validConfig,
				"example.com",
				"user:synthetic-private-value@example.com",
				1,
			),
			err: "without user information",
		},
		"URL fragment": {
			input: strings.Replace(validConfig, "/notify", "/notify#synthetic-private-value", 1),
			err:   "without user information or fragment",
		},
		"invalid port": {
			input: strings.Replace(validConfig, "example.com", "example.com:bad", 1),
			err:   "absolute HTTP(S)",
		},
		"invalid scheme": {
			input: strings.Replace(validConfig, "https:", "ftp:", 1),
			err:   "absolute HTTP(S)",
		},
		"empty host": {
			input: strings.Replace(validConfig, "example.com", "", 1),
			err:   "absolute HTTP(S)",
		},
		"bearer line break": {
			input: validConfig + "    bearer_token: \"synthetic-private-value\\r\\n\"\n",
			err:   "line breaks",
		},
		"command secret": {
			input: strings.Replace(
				validConfig,
				"https://example.com/notify",
				"${cmd:/synthetic-private-value}",
				1,
			),
			err: "only whole env and file",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := readConfig(strings.NewReader(test.input))
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
				assert.Equal(t, Config{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestConfigSecretReferences(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-created")
	tests := map[string]struct {
		value string
		valid bool
	}{
		"environment":         {value: "${env:NOTIFIER_TEST_MISSING}", valid: true},
		"file":                {value: "${file:" + file + "}", valid: true},
		"empty env":           {value: "${env:}"},
		"relative file":       {value: "${file:relative}"},
		"command":             {value: "${cmd:/bin/echo}"},
		"store":               {value: "${store:secrets:key}"},
		"interpolation":       {value: "https://${env:HOST}/notify"},
		"multiple references": {value: "${env:A}${env:B}"},
		"unclosed":            {value: "${env:A"},
		"nested":              {value: "${env:${cmd:/bin/echo}}"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			input := "version: 1\ndestinations:\n  dev:\n    type: webhook\n    url: '" + test.value + "'\n"
			got, err := readConfig(strings.NewReader(input))
			if !test.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, Config{Version: 1, Destinations: map[string]Destination{
				"dev": {Type: "webhook", URL: test.value},
			}}, got)
		})
	}
}

func configForURL(endpoint string) string {
	return fmt.Sprintf(
		"version: 1\ndestinations:\n  dev:\n    type: webhook\n    url: %q\n",
		endpoint,
	)
}
