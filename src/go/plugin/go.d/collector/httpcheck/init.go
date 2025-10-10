// SPDX-License-Identifier: GPL-3.0-or-later

package httpcheck

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

type headerMatch struct {
	exclude    bool
	key        string
	valMatcher matcher.Matcher
}

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("'url' not set")
	}
	return nil
}

func (c *Collector) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(c.ClientConfig)
}

func (c *Collector) initResponseMatchRegexp() (*regexp.Regexp, error) {
	if c.ResponseMatch == "" {
		return nil, nil
	}
	return regexp.Compile(c.ResponseMatch)
}

func (c *Collector) initHeaderMatch() ([]headerMatch, error) {
	if len(c.HeaderMatch) == 0 {
		return nil, nil
	}

	var hms []headerMatch

	for _, v := range c.HeaderMatch {
		if v.Key == "" {
			continue
		}

		hm := headerMatch{
			exclude:    v.Exclude,
			key:        v.Key,
			valMatcher: nil,
		}

		if v.Value != "" {
			m, err := matcher.Parse(v.Value)
			if err != nil {
				return nil, fmt.Errorf("parse key '%s value '%s': %v", v.Key, v.Value, err)
			}
			if v.Exclude {
				m = matcher.Not(m)
			}
			hm.valMatcher = m
		}

		hms = append(hms, hm)
	}

	return hms, nil
}

func (c *Collector) initCharts() *module.Charts {
	charts := httpCheckCharts.Copy()

	for _, chart := range *charts {
		chart.Labels = []module.Label{
			{Key: "url", Value: c.URL},
		}
	}

	return charts
}
