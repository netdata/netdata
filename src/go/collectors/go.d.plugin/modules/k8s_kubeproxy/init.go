// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_kubeproxy

import (
	"errors"

	"github.com/netdata/netdata/go/go.d.plugin/pkg/prometheus"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

func (kp *KubeProxy) validateConfig() error {
	if kp.URL == "" {
		return errors.New("url not set")
	}
	return nil
}

func (kp *KubeProxy) initPrometheusClient() (prometheus.Prometheus, error) {
	httpClient, err := web.NewHTTPClient(kp.Client)
	if err != nil {
		return nil, err
	}

	return prometheus.New(httpClient, kp.Request), nil
}
