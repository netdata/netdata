package geth

import (
	"errors"

	"github.com/netdata/netdata/go/go.d.plugin/pkg/prometheus"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

func (g *Geth) validateConfig() error {
	if g.URL == "" {
		return errors.New("url not set")
	}
	return nil
}

func (g *Geth) initPrometheusClient() (prometheus.Prometheus, error) {
	client, err := web.NewHTTPClient(g.Client)
	if err != nil {
		return nil, err
	}

	return prometheus.New(client, g.Request), nil
}
