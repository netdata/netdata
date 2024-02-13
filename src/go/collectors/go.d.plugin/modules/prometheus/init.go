// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"errors"
	"fmt"
	"os"

	"github.com/netdata/go.d.plugin/pkg/matcher"
	"github.com/netdata/go.d.plugin/pkg/prometheus"
	"github.com/netdata/go.d.plugin/pkg/web"
)

func (p *Prometheus) validateConfig() error {
	if p.URL == "" {
		return errors.New("'url' can not be empty")
	}
	return nil
}

func (p *Prometheus) initPrometheusClient() (prometheus.Prometheus, error) {
	httpClient, err := web.NewHTTPClient(p.Client)
	if err != nil {
		return nil, fmt.Errorf("init HTTP client: %v", err)
	}

	req := p.Request.Copy()
	if p.BearerTokenFile != "" {
		token, err := os.ReadFile(p.BearerTokenFile)
		if err != nil {
			return nil, fmt.Errorf("bearer token file: %v", err)
		}
		req.Headers["Authorization"] = "Bearer " + string(token)
	}

	sr, err := p.Selector.Parse()
	if err != nil {
		return nil, fmt.Errorf("parsing selector: %v", err)
	}

	if sr != nil {
		return prometheus.NewWithSelector(httpClient, req, sr), nil
	}
	return prometheus.New(httpClient, req), nil
}

func (p *Prometheus) initFallbackTypeMatcher(expr []string) (matcher.Matcher, error) {
	if len(expr) == 0 {
		return nil, nil
	}

	m := matcher.FALSE()

	for _, pattern := range expr {
		v, err := matcher.NewGlobMatcher(pattern)
		if err != nil {
			return nil, fmt.Errorf("error on parsing pattern '%s': %v", pattern, err)
		}
		m = matcher.Or(m, v)
	}

	return m, nil
}
