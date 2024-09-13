// SPDX-License-Identifier: GPL-3.0-or-later

package httpcheck

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

type headerMatch struct {
	exclude    bool
	key        string
	valMatcher matcher.Matcher
}

func (hc *HTTPCheck) validateConfig() error {
	if hc.URL == "" {
		return errors.New("'url' not set")
	}
	return nil
}

func (hc *HTTPCheck) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(hc.ClientConfig)
}

func (hc *HTTPCheck) initResponseMatchRegexp() (*regexp.Regexp, error) {
	if hc.ResponseMatch == "" {
		return nil, nil
	}
	return regexp.Compile(hc.ResponseMatch)
}

func (hc *HTTPCheck) initHeaderMatch() ([]headerMatch, error) {
	if len(hc.HeaderMatch) == 0 {
		return nil, nil
	}

	var hms []headerMatch

	for _, v := range hc.HeaderMatch {
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

func (hc *HTTPCheck) initCharts() *module.Charts {
	charts := httpCheckCharts.Copy()

	for _, chart := range *charts {
		chart.Labels = []module.Label{
			{Key: "url", Value: hc.URL},
		}
	}

	return charts
}
