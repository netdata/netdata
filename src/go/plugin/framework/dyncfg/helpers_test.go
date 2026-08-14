// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommandFromArgs(t *testing.T) {
	tests := map[string]struct {
		args []string
		want Command
	}{
		"missing": {
			want: "",
		},
		"missing command": {
			args: []string{"id"},
			want: "",
		},
		"lowercase": {
			args: []string{"id", "enable"},
			want: CommandEnable,
		},
		"uppercase": {
			args: []string{"id", "ENABLE"},
			want: CommandEnable,
		},
		"unknown": {
			args: []string{"id", "CUSTOM"},
			want: "custom",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.want, CommandFromArgs(test.args))
		})
	}
}

func TestNormalizeJobName(t *testing.T) {
	tests := map[string]struct {
		name string
		want string
	}{
		"empty": {},
		"unchanged": {
			name: "collector.job",
			want: "collector.job",
		},
		"spaces": {
			name: "collector job",
			want: "collector_job",
		},
		"colons": {
			name: "collector:job",
			want: "collector_job",
		},
		"combined": {
			name: "collector job:instance",
			want: "collector_job_instance",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.want, NormalizeJobName(test.name))
		})
	}
}
