// SPDX-License-Identifier: GPL-3.0-or-later

package bind

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

func (b *Bind) validateConfig() error {
	if b.URL == "" {
		return errors.New("url not set")
	}
	return nil
}

func (b *Bind) initPermitViewMatcher() (matcher.Matcher, error) {
	if b.PermitView == "" {
		return nil, nil
	}
	return matcher.NewSimplePatternsMatcher(b.PermitView)
}

func (b *Bind) initBindApiClient(httpClient *http.Client) (bindAPIClient, error) {
	switch {
	case strings.HasSuffix(b.URL, "/xml/v3"): // BIND 9.9+
		return newXML3Client(httpClient, b.RequestConfig), nil
	case strings.HasSuffix(b.URL, "/json/v1"): // BIND 9.10+
		return newJSONClient(httpClient, b.RequestConfig), nil
	default:
		return nil, fmt.Errorf("URL %s is wrong, supported endpoints: `/xml/v3`, `/json/v1`", b.URL)
	}
}
