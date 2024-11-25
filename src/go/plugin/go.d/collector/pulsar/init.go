// SPDX-License-Identifier: GPL-3.0-or-later

package pulsar

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (p *Pulsar) validateConfig() error {
	if p.URL == "" {
		return errors.New("url not set")
	}
	return nil
}

func (p *Pulsar) initPrometheusClient() (prometheus.Prometheus, error) {
	client, err := web.NewHTTPClient(p.ClientConfig)
	if err != nil {
		return nil, err
	}

	return prometheus.New(client, p.RequestConfig), nil
}

func (p *Pulsar) initTopicFilerMatcher() (matcher.Matcher, error) {
	if p.TopicFilter.Empty() {
		return matcher.FALSE(), nil
	}
	return p.TopicFilter.Parse()
}
