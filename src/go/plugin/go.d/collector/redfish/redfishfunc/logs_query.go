// SPDX-License-Identifier: GPL-3.0-or-later

package redfishfunc

import (
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

type logQuery struct {
	service, severity string
	after, before     *int64
	text              matcher.Matcher
}

func parseLogQuery(req funcapi.RawMethodRequest, now time.Time) (logQuery, error) {
	params, err := funcapi.ParseRawParams(req)
	if err != nil {
		return logQuery{}, err
	}
	var q logQuery
	if q.service, err = params.String("service"); err != nil {
		return q, err
	}
	if q.service == "" {
		return q, errors.New("select a log service")
	}
	if q.severity, err = params.String("severity"); err != nil {
		return q, err
	}
	switch q.severity {
	case "", "all", "OK", "Warning", "Critical", "Unknown":
	default:
		return q, errors.New("invalid severity selection")
	}
	if q.after, q.before, err = params.TimeRange(now); err != nil {
		return q, err
	}
	if q.text, err = params.QueryMatcher(); err != nil {
		return q, err
	}
	return q, nil
}
