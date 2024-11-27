// SPDX-License-Identifier: GPL-3.0-or-later

package bind

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("url not set")
	}
	return nil
}

func (c *Collector) initPermitViewMatcher() (matcher.Matcher, error) {
	if c.PermitView == "" {
		return nil, nil
	}
	return matcher.NewSimplePatternsMatcher(c.PermitView)
}

func (c *Collector) initBindApiClient(httpClient *http.Client) (bindAPIClient, error) {
	switch {
	case strings.HasSuffix(c.URL, "/xml/v3"): // BIND 9.9+
		return newXML3Client(httpClient, c.RequestConfig), nil
	case strings.HasSuffix(c.URL, "/json/v1"): // BIND 9.10+
		return newJSONClient(httpClient, c.RequestConfig), nil
	default:
		return nil, fmt.Errorf("URL %s is wrong, supported endpoints: `/xml/v3`, `/json/v1`", c.URL)
	}
}
