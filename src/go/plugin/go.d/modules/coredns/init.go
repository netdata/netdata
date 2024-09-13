// SPDX-License-Identifier: GPL-3.0-or-later

package coredns

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (cd *CoreDNS) validateConfig() error {
	if cd.URL == "" {
		return errors.New("url not set")
	}
	return nil
}

func (cd *CoreDNS) initPerServerMatcher() (matcher.Matcher, error) {
	if cd.PerServerStats.Empty() {
		return nil, nil
	}
	return cd.PerServerStats.Parse()
}

func (cd *CoreDNS) initPerZoneMatcher() (matcher.Matcher, error) {
	if cd.PerZoneStats.Empty() {
		return nil, nil
	}
	return cd.PerZoneStats.Parse()
}

func (cd *CoreDNS) initPrometheusClient() (prometheus.Prometheus, error) {
	client, err := web.NewHTTPClient(cd.ClientConfig)
	if err != nil {
		return nil, err
	}
	return prometheus.New(client, cd.RequestConfig), nil
}
