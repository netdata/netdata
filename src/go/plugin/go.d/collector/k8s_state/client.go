// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import (
	"errors"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/mattn/go-isatty"
)

const (
	envKubeServiceHost = "KUBERNETES_SERVICE_HOST"
	envKubeServicePort = "KUBERNETES_SERVICE_PORT"
)

func newKubeClient() (kubernetes.Interface, error) {
	if os.Getenv(envKubeServiceHost) != "" && os.Getenv(envKubeServicePort) != "" {
		return newKubeClientInCluster()
	}
	if isatty.IsTerminal(os.Stdout.Fd()) {
		return newKubeClientOutOfCluster()
	}
	return nil, errors.New("can not create Kubernetes client: not inside a cluster")
}

func newKubeClientInCluster() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	config.UserAgent = "Netdata/kube-state"
	return kubernetes.NewForConfig(config)
}

func newKubeClientOutOfCluster() (*kubernetes.Clientset, error) {
	home := homeDir()
	if home == "" {
		return nil, errors.New("couldn't find home directory")
	}

	configPath := filepath.Join(home, ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", configPath)
	if err != nil {
		return nil, err
	}

	config.UserAgent = "Netdata/kube-state"
	return kubernetes.NewForConfig(config)
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
