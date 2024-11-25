package geth

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (g *Geth) validateConfig() error {
	if g.URL == "" {
		return errors.New("url not set")
	}
	return nil
}

func (g *Geth) initPrometheusClient() (prometheus.Prometheus, error) {
	client, err := web.NewHTTPClient(g.ClientConfig)
	if err != nil {
		return nil, err
	}

	return prometheus.New(client, g.RequestConfig), nil
}
