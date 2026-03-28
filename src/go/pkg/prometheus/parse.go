package prometheus

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
)

const (
	quantileLabel = "quantile"
	bucketLabel   = "le"
)

const (
	countSuffix  = "_count"
	sumSuffix    = "_sum"
	bucketSuffix = "_bucket"
)

type promTextParser struct {
	series     Series
	sr         selector.Selector
	currSeries labels.Labels
}

func (p *promTextParser) parseToSeries(text []byte) (Series, error) {
	p.series.Reset()

	parser := textparse.NewPromParser(text, labels.NewSymbolTable())
	for {
		entry, err := parser.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if entry == textparse.EntryInvalid && strings.HasPrefix(err.Error(), "invalid metric type") {
				continue
			}
			return nil, fmt.Errorf("failed to parse prometheus metrics: %v", err)
		}

		switch entry {
		case textparse.EntrySeries:
			p.currSeries = p.currSeries[:0]

			parser.Metric(&p.currSeries)

			if p.sr != nil && !p.sr.Matches(p.currSeries) {
				continue
			}

			_, _, val := parser.Series()
			p.series.Add(SeriesSample{Labels: copyLabels(p.currSeries), Value: val})
		}
	}

	p.series.Sort()

	return p.series, nil
}

var reSpace = regexp.MustCompile(`\s+`)

func copyLabels(lbs []labels.Label) []labels.Label {
	return append([]labels.Label(nil), lbs...)
}

func removeLabel(lbs labels.Labels, name string) (labels.Labels, string, bool) {
	for i, v := range lbs {
		if v.Name == name {
			return append(lbs[:i], lbs[i+1:]...), v.Value, true
		}
	}
	return lbs, "", false
}

func metricNameValue(lbs labels.Labels) (string, bool) {
	for _, v := range lbs {
		if v.Name == labels.MetricName {
			return v.Value, true
		}
	}
	return "", false
}
