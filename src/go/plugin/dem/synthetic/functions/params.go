// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/query"
)

func permitted(value string, required uint64) bool {
	mask, err := strconv.ParseUint(value, 0, 64)
	return err == nil && mask&required == required
}

func parseArgs(words, accepted []string) (map[string]string, error) {
	args := make(map[string]string, len(words))
	for _, word := range words {
		key, value, ok := strings.Cut(word, ":")
		if !ok {
			return nil, errors.New("expected key:value argument")
		}
		allowed := false
		for _, name := range accepted {
			if key == name {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("unsupported argument %q", key)
		}
		if _, exists := args[key]; exists {
			return nil, fmt.Errorf("duplicate argument %q", key)
		}
		args[key] = value
	}
	return args, nil
}

func rowLimit(value string) (int, error) {
	if value == "" {
		return 2000, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > 2000 {
		return 0, errors.New("limit must be between 1 and 2000")
	}
	return limit, nil
}

func validKind(value string) error {
	if value != "" && value != string(model.Journey) && value != string(model.Lighthouse) {
		return errors.New("kind must be journey or lighthouse")
	}
	return nil
}

func runFilter(args map[string]string, now int64) (query.RunFilter, error) {
	before := now
	f := query.RunFilter{
		JobID:   args["job_id"],
		Kind:    model.Kind(args["kind"]),
		Outcome: model.Outcome(args["outcome"]),
		Before:  &before,
	}
	if err := validKind(args["kind"]); err != nil {
		return f, err
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
	for key, dst := range map[string]*int64{"after": &f.After, "before": f.Before} {
		if args[key] != "" {
			v, err := strconv.ParseInt(args[key], 10, 64)
			if err != nil {
				return f, fmt.Errorf("%s must be Unix seconds or a relative negative offset", key)
			}
			if v < 0 {
				v += now
			}
			if v < 0 {
				return f, fmt.Errorf("%s must not precede the Unix epoch", key)
			}
			*dst = v
		}
	}
	if f.After > *f.Before {
		return f, errors.New("after must not exceed before")
	}
	return f, nil
}
