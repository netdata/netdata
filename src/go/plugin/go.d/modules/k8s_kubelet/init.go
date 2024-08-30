// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_kubelet

import (
	"errors"
	"os"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (k *Kubelet) validateConfig() error {
	if k.URL == "" {
		return errors.New("url not set")
	}
	return nil
}

func (k *Kubelet) initAuthToken() string {
	bs, err := os.ReadFile(k.TokenPath)
	if err != nil {
		k.Warningf("error on reading service account token from '%s': %v", k.TokenPath, err)
	}
	return string(bs)
}

func (k *Kubelet) initPrometheusClient() (prometheus.Prometheus, error) {
	httpClient, err := web.NewHTTPClient(k.Client)
	if err != nil {
		return nil, err
	}

	return prometheus.New(httpClient, k.Request), nil
}
