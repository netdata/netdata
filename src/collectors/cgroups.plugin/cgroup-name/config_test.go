// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"os"
	"testing"
)

func TestInvocationConfigPreservesKubeConfigPresence(t *testing.T) {
	original, wasSet := os.LookupEnv("KUBE_CONFIG")
	t.Cleanup(func() {
		if wasSet {
			_ = os.Setenv("KUBE_CONFIG", original)
		} else {
			_ = os.Unsetenv("KUBE_CONFIG")
		}
	})

	tests := map[string]struct {
		set       bool
		value     string
		wantSet   bool
		wantValue string
	}{
		"unset remains distinguishable": {},
		"explicitly empty remains set": {
			set:     true,
			wantSet: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.set {
				_ = os.Setenv("KUBE_CONFIG", test.value)
			} else {
				_ = os.Unsetenv("KUBE_CONFIG")
			}
			config := loadInvocationConfig()
			if config.kubernetes.kubeConfigSet != test.wantSet || config.kubernetes.kubeConfig != test.wantValue {
				t.Fatalf("KUBE_CONFIG: set=%v value=%q, want set=%v value=%q",
					config.kubernetes.kubeConfigSet, config.kubernetes.kubeConfig, test.wantSet, test.wantValue)
			}
		})
	}
}

func TestPrepareInvocationConfigDefaultsRuntimeHosts(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("PODMAN_HOST", "")

	config := prepareInvocationConfig()
	tests := map[string]struct {
		envName     string
		configValue string
		want        string
	}{
		"docker host": {
			envName:     "DOCKER_HOST",
			configValue: config.dockerHost,
			want:        defaultDockerHost,
		},
		"podman host": {
			envName:     "PODMAN_HOST",
			configValue: config.podmanHost,
			want:        defaultPodmanHost,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.configValue != test.want {
				t.Fatalf("config value = %q, want %q", test.configValue, test.want)
			}
			if got := os.Getenv(test.envName); got != test.want {
				t.Fatalf("exported %s = %q, want %q", test.envName, got, test.want)
			}
		})
	}
}

func TestParseLogPriorityMatchesShellAliases(t *testing.T) {
	tests := map[string]struct {
		value string
		want  int
	}{
		"emerg":     {value: "emerg", want: 0},
		"emergency": {value: "emergency", want: 0},
		"alert":     {value: "alert", want: 1},
		"crit":      {value: "crit", want: 2},
		"critical":  {value: "critical", want: 2},
		"err":       {value: "err", want: 3},
		"error":     {value: "error", want: 3},
		"warn":      {value: "warn", want: 4},
		"warning":   {value: "warning", want: 4},
		"notice":    {value: "notice", want: 5},
		"info":      {value: "info", want: 6},
		"debug":     {value: "debug", want: 7},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := parseLogPriority(test.value); got != test.want {
				t.Fatalf("parseLogPriority(%q) = %d, want %d", test.value, got, test.want)
			}
		})
	}
}
