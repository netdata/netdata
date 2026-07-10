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

	_ = os.Unsetenv("KUBE_CONFIG")
	if config := loadInvocationConfig(); config.kubernetes.kubeConfigSet {
		t.Fatal("unset KUBE_CONFIG must remain distinguishable from an empty value")
	}

	_ = os.Setenv("KUBE_CONFIG", "")
	config := loadInvocationConfig()
	if !config.kubernetes.kubeConfigSet || config.kubernetes.kubeConfig != "" {
		t.Fatalf("explicitly empty KUBE_CONFIG was not preserved: set=%v value=%q",
			config.kubernetes.kubeConfigSet, config.kubernetes.kubeConfig)
	}
}

func TestPrepareInvocationConfigDefaultsRuntimeHosts(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("PODMAN_HOST", "")

	config := prepareInvocationConfig()
	if config.dockerHost != defaultDockerHost {
		t.Fatalf("DOCKER_HOST = %q, want %q", config.dockerHost, defaultDockerHost)
	}
	if config.podmanHost != defaultPodmanHost {
		t.Fatalf("PODMAN_HOST = %q, want %q", config.podmanHost, defaultPodmanHost)
	}
	if got := os.Getenv("DOCKER_HOST"); got != defaultDockerHost {
		t.Fatalf("DOCKER_HOST was not exported for child commands: %q", got)
	}
	if got := os.Getenv("PODMAN_HOST"); got != defaultPodmanHost {
		t.Fatalf("PODMAN_HOST was not exported for child commands: %q", got)
	}
}
