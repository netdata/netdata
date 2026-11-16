// SPDX-License-Identifier: GPL-3.0-or-later

package syntheticfunc

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/rawfunc"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
)

// limitHelp describes the limit argument of rowLimit and the truncated response field.
var limitHelp = "Pass limit (1 to " + strconv.Itoa(rawfunc.MaxRows) + ", default " +
	strconv.Itoa(rawfunc.MaxRows) + ") to cap the rows; truncated is true when more match."

func rowLimit(value string) (int, error) {
	if value == "" {
		return rawfunc.MaxRows, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > rawfunc.MaxRows {
		return 0, fmt.Errorf("limit must be between 1 and %d", rawfunc.MaxRows)
	}
	return limit, nil
}

// runFilter selects one kind's runs from the history method arguments.
func runFilter(kind model.Kind, args map[string]string, now int64) (history.RunFilter, error) {
	before := now
	f := history.RunFilter{
		JobID:     args["job_id"],
		VnodeGUID: args["vnode_guid"],
		Kind:      kind,
		Outcome:   model.Outcome(args["outcome"]),
		Before:    &before,
	}
	switch f.Outcome {
	case "",
		model.Unknown,
		model.Success,
		model.Failed,
		model.Timeout,
		model.Inconclusive,
		model.Error,
		model.Cancelled:
	default:
		return f, errors.New("unknown outcome filter")
	}
	var err error
	f.Limit, err = rowLimit(args["limit"])
	if err != nil {
		return f, err
	}
	for _, bound := range []struct {
		key string
		dst *int64
	}{{"after", &f.After}, {"before", f.Before}} {
		if args[bound.key] != "" {
			v, err := strconv.ParseInt(args[bound.key], 10, 64)
			if err != nil {
				return f, fmt.Errorf("%s must be Unix seconds or a relative negative offset", bound.key)
			}
			if v < 0 {
				v += now
			}
			if v < 0 {
				return f, fmt.Errorf("%s must not precede the Unix epoch", bound.key)
			}
			*bound.dst = v
		}
	}
	if f.After > *f.Before {
		return f, errors.New("after must not exceed before")
	}
	return f, nil
}
