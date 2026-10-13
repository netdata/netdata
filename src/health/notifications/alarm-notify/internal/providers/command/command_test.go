// SPDX-License-Identifier: GPL-3.0-or-later

package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandConfig(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	for name, test := range map[string]struct {
		change func(*Config)
		err    string
	}{
		"defaults":     {},
		"literal args": {change: func(d *Config) { d.Args = []string{"", "a b", "$(literal)", "${env:LITERAL}", "line\nbreak"} }},
		"explicit env": {change: func(d *Config) {
			d.Env = map[string]string{"EMPTY": "", "TOKEN": "${env:UNREAD_COMMAND_SECRET}", "PATH": "/usr/bin:/bin", "_1": "x"}
		}},
		"file reference": {change: func(d *Config) {
			d.Env = map[string]string{"TOKEN": "${file:" + filepath.Join(t.TempDir(), "unread") + "}"}
		}},
		"missing executable":    {change: func(d *Config) { d.Executable = "" }, err: "absolute path"},
		"relative executable":   {change: func(d *Config) { d.Executable = "synthetic-private-value" }, err: "absolute path"},
		"executable reference":  {change: func(d *Config) { d.Executable = "${env:UNREAD_COMMAND}" }, err: "literal"},
		"executable NUL":        {change: func(d *Config) { d.Executable += "\x00synthetic-private-value" }, err: "NUL"},
		"argument NUL":          {change: func(d *Config) { d.Args = []string{"synthetic-private-value\x00"} }, err: "NUL"},
		"empty env name":        {change: func(d *Config) { d.Env = map[string]string{"": "synthetic-private-value"} }, err: "env names"},
		"leading digit":         {change: func(d *Config) { d.Env = map[string]string{"1TOKEN": "x"} }, err: "env names"},
		"env equals":            {change: func(d *Config) { d.Env = map[string]string{"KEY=synthetic-private-value": "x"} }, err: "env names"},
		"env NUL":               {change: func(d *Config) { d.Env = map[string]string{"TOKEN": "synthetic-private-value\x00"} }, err: "NUL"},
		"invalid env reference": {change: func(d *Config) { d.Env = map[string]string{"TOKEN": "${cmd:synthetic-private-value}"} }, err: "env and file"},
	} {
		t.Run(name, func(t *testing.T) {
			dst := Config{Executable: executable}
			if test.change != nil {
				test.change(&dst)
			}
			checkConfig(t, dst, test.err)
		})
	}
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
		return Config{Executable: "/usr/bin/helper", Args: []string{"literal"}, Env: map[string]string{"TOKEN": "literal"}}
	}
	for name, mutate := range map[string]func(*Config){
		"arguments":   func(c *Config) { c.Args[0] = "changed" },
		"environment": func(c *Config) { c.Env["TOKEN"] = "changed" },
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
