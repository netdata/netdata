// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultDockerHost          = "unix:///var/run/docker.sock"
	defaultPodmanHost          = "unix:///run/podman/podman.sock"
	defaultKubeletURL          = "https://localhost:10250"
	defaultGCPMetadataURL      = "http://metadata"
	k8sServiceAccountCAFile    = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	k8sServiceAccountTokenFile = "/var/run/secrets/kubernetes.io/serviceaccount/token"
)

// sbindirPost is set by CMake through -X main.sbindirPost=... . Keep this
// symbol in package main: its fully qualified name is part of the build.
var sbindirPost string

type invocationConfig struct {
	dockerHost string
	podmanHost string
	hostPrefix string
	tmpDir     string
	logLevel   int
	timeout    time.Duration
	kubernetes kubernetesConfig
}

type kubernetesConfig struct {
	serviceHost             string
	servicePort             string
	serviceAccountCAFile    string
	serviceAccountTokenFile string
	useKubelet              bool
	kubeletURL              string
	nodeName                string
	kubeConfig              string
	kubeConfigSet           bool
	tlsInsecure             bool
	gcpMetadataURL          string
}

func prepareInvocationConfig() invocationConfig {
	setupEnvironment()
	setDefaultEnv("DOCKER_HOST", defaultDockerHost)
	setDefaultEnv("PODMAN_HOST", defaultPodmanHost)
	return loadInvocationConfig()
}

func loadInvocationConfig() invocationConfig {
	tmpDir := os.Getenv("TMPDIR")
	if tmpDir == "" {
		tmpDir = "/tmp"
	}

	ms, _ := strconv.ParseInt(os.Getenv("NETDATA_CGROUP_NAME_TIMEOUT_MS"), 10, 64)
	var timeout time.Duration
	if ms > 0 {
		timeout = time.Duration(ms) * time.Millisecond
	}

	kubeConfig, kubeConfigSet := os.LookupEnv("KUBE_CONFIG")
	kubeletURL := os.Getenv("KUBELET_URL")
	if kubeletURL == "" {
		kubeletURL = defaultKubeletURL
	}

	return invocationConfig{
		dockerHost: os.Getenv("DOCKER_HOST"),
		podmanHost: os.Getenv("PODMAN_HOST"),
		hostPrefix: os.Getenv("NETDATA_HOST_PREFIX"),
		tmpDir:     tmpDir,
		logLevel:   parseLogPriority(os.Getenv("NETDATA_LOG_LEVEL")),
		timeout:    timeout,
		kubernetes: kubernetesConfig{
			serviceHost:             os.Getenv("KUBERNETES_SERVICE_HOST"),
			servicePort:             os.Getenv("KUBERNETES_PORT_443_TCP_PORT"),
			serviceAccountCAFile:    k8sServiceAccountCAFile,
			serviceAccountTokenFile: k8sServiceAccountTokenFile,
			useKubelet:              os.Getenv("USE_KUBELET_FOR_PODS_METADATA") != "",
			kubeletURL:              kubeletURL,
			nodeName:                os.Getenv("MY_NODE_NAME"),
			kubeConfig:              kubeConfig,
			kubeConfigSet:           kubeConfigSet,
			tlsInsecure:             parseK8sTLSInsecure(os.Getenv("K8S_TLS_INSECURE")),
			gcpMetadataURL:          defaultGCPMetadataURL,
		},
	}
}

// setupEnvironment extends the inherited PATH rather than replacing it so the
// same host-installed docker, kubectl, snap, and ps commands remain discoverable.
func setupEnvironment() {
	path := os.Getenv("PATH") + ":/sbin:/usr/sbin:/usr/local/sbin"
	if sbindirPost != "" {
		path += ":" + sbindirPost
	}
	_ = os.Setenv("PATH", path)
	_ = os.Setenv("LC_ALL", "C")
}

func setDefaultEnv(name, value string) {
	if os.Getenv(name) == "" {
		_ = os.Setenv(name, value)
	}
}

func parseLogPriority(value string) int {
	switch strings.ToLower(value) {
	case "emerg", "emergency":
		return ndlpEmerg
	case "alert":
		return ndlpAlert
	case "crit", "critical":
		return ndlpCrit
	case "err", "error":
		return ndlpErr
	case "warn", "warning":
		return ndlpWarn
	case "notice":
		return ndlpNotice
	case "debug":
		return ndlpDebug
	default:
		return ndlpInfo
	}
}

func parseK8sTLSInsecure(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "0", "false", "no":
		return false
	default:
		return true
	}
}
