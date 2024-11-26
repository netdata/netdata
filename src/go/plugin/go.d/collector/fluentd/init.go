// SPDX-License-Identifier: GPL-3.0-or-later

package fluentd

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (f *Fluentd) validateConfig() error {
	if f.URL == "" {
		return errors.New("url not set")
	}

	return nil
}

func (f *Fluentd) initPermitPluginMatcher() (matcher.Matcher, error) {
	if f.PermitPlugin == "" {
		return matcher.TRUE(), nil
	}

	return matcher.NewSimplePatternsMatcher(f.PermitPlugin)
}

func (f *Fluentd) initApiClient() (*apiClient, error) {
	client, err := web.NewHTTPClient(f.ClientConfig)
	if err != nil {
		return nil, err
	}

	return newAPIClient(client, f.RequestConfig), nil
}
