// SPDX-License-Identifier: GPL-3.0-or-later

package promscrapemodel

import (
	"strings"

	"github.com/prometheus/prometheus/model/labels"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/promselector"
)

const (
	quantileLabel = "quantile"
	bucketLabel   = "le"
	countSuffix   = "_count"
	sumSuffix     = "_sum"
	bucketSuffix  = "_bucket"
)

type Parser struct {
	driver parseDriver
}

func NewParser(sr promselector.Selector) Parser {
	return Parser{driver: parseDriver{sr: sr}}
}

func (p *Parser) ParseStream(text []byte, onSample func(Sample) error) error {
	if onSample == nil {
		return nil
	}
	return p.driver.parse(text, true, nil, onSample)
}

func (p *Parser) ParseStreamWithMeta(text []byte, onHelp func(name, help string), onSample func(Sample) error) error {
	if onSample == nil {
		return nil
	}
	return p.driver.parse(text, true, onHelp, onSample)
}

func copyLabelsExcept(lbs labels.Labels, skip string) []labels.Label {
	out := make([]labels.Label, 0, len(lbs))
	for _, lb := range lbs {
		if lb.Name == skip {
			continue
		}
		out = append(out, lb)
	}
	return out
}

func metricNameValue(lbs labels.Labels) (string, bool) {
	for _, v := range lbs {
		if v.Name == labels.MetricName {
			return v.Value, true
		}
	}
	return "", false
}

func sanitizeHelp(help string) string {
	if strings.IndexByte(help, '\n') == -1 {
		return help
	}
	return strings.Join(strings.Fields(help), " ")
}
