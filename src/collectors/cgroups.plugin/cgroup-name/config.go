// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"os"
	"path/filepath"
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
	// Commands are executed by basename, so this list must never inherit caller-controlled entries.
	trustedExecutablePath = "/bin:/sbin:/usr/bin:/usr/sbin:/usr/local/bin:/usr/local/sbin:/snap/bin"
)

type invocationConfig struct {
	dockerHost         string
	podmanHost         string
	hostPrefix         string
	kubernetesCacheDir string
	logLevel           int
	timeout            time.Duration
	kubernetes         kubernetesConfig
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
	kubernetesCacheDir := cgroupNameStateDir(os.Getenv("NETDATA_LIB_DIR"), os.Getenv("TMPDIR"), os.Geteuid())

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
		dockerHost:         os.Getenv("DOCKER_HOST"),
		podmanHost:         os.Getenv("PODMAN_HOST"),
		hostPrefix:         os.Getenv("NETDATA_HOST_PREFIX"),
		kubernetesCacheDir: kubernetesCacheDir,
		logLevel:           parseLogPriority(os.Getenv("NETDATA_LOG_LEVEL")),
		timeout:            timeout,
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

func cgroupNameStateDir(varLibDir, tmpDir string, euid int) string {
	if varLibDir = strings.TrimSpace(varLibDir); varLibDir != "" {
		return filepath.Join(varLibDir, "cgroup-name")
	}
	if tmpDir = strings.TrimSpace(tmpDir); tmpDir == "" {
		tmpDir = os.TempDir()
	}
	return filepath.Join(tmpDir, fmt.Sprintf("netdata-cgroup-name-%d", euid))
}

func setupEnvironment() {
	_ = os.Setenv("PATH", trustedExecutablePath)
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
