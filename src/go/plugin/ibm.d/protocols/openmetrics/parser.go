package openmetrics

import (
	"fmt"
	"io"

	promLabels "github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
)

type seriesParser struct {
	selector selector.Selector
}

func (p *seriesParser) parse(data []byte) (prometheus.Series, error) {
	var series prometheus.Series
	parser := textparse.NewPromParser(data, promLabels.NewSymbolTable())
	var tmp promLabels.Labels

	for {
		entry, err := parser.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			if entry == textparse.EntryInvalid {
				// Skip invalid metric types and continue.
				continue
			}
			return nil, fmt.Errorf("openmetrics parser: failed to parse metrics: %w", err)
		}

		if entry != textparse.EntrySeries {
			continue
		}

		tmp = tmp[:0]
		parser.Metric(&tmp)

		if p.selector != nil && !p.selector.Matches(tmp) {
			continue
		}

		_, _, value := parser.Series()
		labels := append(promLabels.Labels(nil), tmp...)
		series.Add(prometheus.SeriesSample{Labels: labels, Value: value})
	}

	series.Sort()
	return series, nil
}
