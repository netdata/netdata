// SPDX-License-Identifier: GPL-3.0-or-later

package k8sclient

import (
	"errors"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

const (
	EnvFakeClient    = "KUBERNETES_FAKE_CLIENTSET"
	defaultUserAgent = "Netdata/k8s-client"
)

func New(userAgent string) (kubernetes.Interface, error) {
	if userAgent == "" {
		userAgent = defaultUserAgent
	}

	switch {
	case os.Getenv(EnvFakeClient) != "":
		return fake.NewClientset(), nil
	case os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_SERVICE_PORT") != "":
		return newInCluster(userAgent)
	default:
		return newOutOfCluster(userAgent)
	}
}

func newInCluster(userAgent string) (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	config.UserAgent = userAgent

	return kubernetes.NewForConfig(config)
}

func newOutOfCluster(userAgent string) (*kubernetes.Clientset, error) {
	home := homeDir()
	if home == "" {
		return nil, errors.New("couldn't find home directory")
	}

	path := filepath.Join(home, ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		return nil, err
	}

	config.UserAgent = userAgent

	return kubernetes.NewForConfig(config)
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
