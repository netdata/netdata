// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnboundConfig_Empty(t *testing.T) {
	assert.True(t, UnboundConfig{}.Empty())
	assert.False(t, UnboundConfig{enable: "yes"}.Empty())
}

func TestUnboundConfig_Cumulative(t *testing.T) {
	tests := []struct {
		input     string
		wantValue bool
		wantOK    bool
	}{
		{input: "yes", wantValue: true, wantOK: true},
		{input: "no", wantValue: false, wantOK: true},
		{input: "", wantValue: false, wantOK: false},
		{input: "some value", wantValue: false, wantOK: true},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			cfg := UnboundConfig{cumulative: test.input}

			v, ok := cfg.Cumulative()
			assert.Equal(t, test.wantValue, v)
			assert.Equal(t, test.wantOK, ok)
		})
	}
}

func TestUnboundConfig_ControlEnabled(t *testing.T) {
	tests := []struct {
		input     string
		wantValue bool
		wantOK    bool
	}{
		{input: "yes", wantValue: true, wantOK: true},
		{input: "no", wantValue: false, wantOK: true},
		{input: "", wantValue: false, wantOK: false},
		{input: "some value", wantValue: false, wantOK: true},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			cfg := UnboundConfig{enable: test.input}

			v, ok := cfg.ControlEnabled()
			assert.Equal(t, test.wantValue, v)
			assert.Equal(t, test.wantOK, ok)
		})
	}
}

func TestUnboundConfig_ControlInterface(t *testing.T) {
	tests := []struct {
		input     string
		wantValue string
		wantOK    bool
	}{
		{input: "127.0.0.1", wantValue: "127.0.0.1", wantOK: true},
		{input: "/var/run/unbound.sock", wantValue: "/var/run/unbound.sock", wantOK: true},
		{input: "", wantValue: "", wantOK: false},
		{input: "some value", wantValue: "some value", wantOK: true},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			cfg := UnboundConfig{iface: test.input}

			v, ok := cfg.ControlInterface()
			assert.Equal(t, test.wantValue, v)
			assert.Equal(t, test.wantOK, ok)
		})
	}
}

func TestUnboundConfig_ControlPort(t *testing.T) {
	tests := []struct {
		input     string
		wantValue string
		wantOK    bool
	}{
		{input: "8953", wantValue: "8953", wantOK: true},
		{input: "", wantValue: "", wantOK: false},
		{input: "some value", wantValue: "some value", wantOK: true},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			cfg := UnboundConfig{port: test.input}

			v, ok := cfg.ControlPort()
			assert.Equal(t, test.wantValue, v)
			assert.Equal(t, test.wantOK, ok)
		})
	}
}

func TestUnboundConfig_ControlUseCert(t *testing.T) {
	tests := []struct {
		input     string
		wantValue bool
		wantOK    bool
	}{
		{input: "yes", wantValue: true, wantOK: true},
		{input: "no", wantValue: false, wantOK: true},
		{input: "", wantValue: false, wantOK: false},
		{input: "some value", wantValue: false, wantOK: true},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			cfg := UnboundConfig{useCert: test.input}

			v, ok := cfg.ControlUseCert()
			assert.Equal(t, test.wantValue, v)
			assert.Equal(t, test.wantOK, ok)
		})
	}
}

func TestUnboundConfig_ControlKeyFile(t *testing.T) {
	tests := []struct {
		input     string
		wantValue string
		wantOK    bool
	}{
		{input: "/etc/unbound/unbound_control.key", wantValue: "/etc/unbound/unbound_control.key", wantOK: true},
		{input: "", wantValue: "", wantOK: false},
		{input: "some value", wantValue: "some value", wantOK: true},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			cfg := UnboundConfig{keyFile: test.input}

			v, ok := cfg.ControlKeyFile()
			assert.Equal(t, test.wantValue, v)
			assert.Equal(t, test.wantOK, ok)
		})
	}
}

func TestUnboundConfig_ControlCertFile(t *testing.T) {
	tests := []struct {
		input     string
		wantValue string
		wantOK    bool
	}{
		{input: "/etc/unbound/unbound_control.pem", wantValue: "/etc/unbound/unbound_control.pem", wantOK: true},
		{input: "", wantValue: "", wantOK: false},
		{input: "some value", wantValue: "some value", wantOK: true},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			cfg := UnboundConfig{certFile: test.input}

			v, ok := cfg.ControlCertFile()
			assert.Equal(t, test.wantValue, v)
			assert.Equal(t, test.wantOK, ok)
		})
	}
}
