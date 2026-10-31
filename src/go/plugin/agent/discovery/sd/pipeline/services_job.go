// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

// jobError contains only a safe reason, never submitted values or template source.
type jobError string

func (e jobError) Error() string { return string(e) }

func (r *serviceRule) execute(buf *bytes.Buffer, target model.Target) ([]confgroup.Config, error) {
	r.jobs = nil
	defer func() { r.jobs = nil }()

	if err := r.tmpl.Execute(buf, target); err != nil {
		return nil, err
	}
	if len(r.jobs) > 0 && strings.TrimSpace(buf.String()) != "" {
		return nil, jobError("job: cannot mix submitted jobs with rendered text")
	}
	return r.jobs, nil
}

func (r *serviceRule) emitJob(args ...any) (string, error) {
	var config confgroup.Config
	if len(args) == 1 {
		switch value := args[0].(type) {
		case confgroup.Config:
			config = value
		case map[string]any:
			config = confgroup.Config(value)
		case map[any]any:
			if value != nil {
				config = make(confgroup.Config, len(value))
				for key, value := range value {
					key, ok := key.(string)
					if !ok {
						return "", jobError("job: map keys must be strings")
					}
					config[key] = value
				}
			}
		default:
			return "", jobError("job: expected a map or key/value pairs")
		}
		if config == nil {
			return "", jobError("job: map must not be nil")
		}
	} else {
		if len(args) == 0 || len(args)%2 != 0 {
			return "", jobError("job: expected a map or complete key/value pairs")
		}
		config = make(confgroup.Config, len(args)/2)
		for i := 0; i < len(args); i += 2 {
			key, ok := args[i].(string)
			if !ok {
				return "", jobError(fmt.Sprintf("job: argument %d must be a string key", i+1))
			}
			if _, exists := config[key]; exists {
				return "", jobError(fmt.Sprintf("job: argument %d repeats a key", i+1))
			}
			config[key] = args[i+1]
		}
	}

	// Freeze values at the call boundary and retain the legacy YAML config types.
	snapshot, err := config.Clone()
	if err != nil {
		return "", jobError("job: could not serialize the configuration")
	}
	r.jobs = append(r.jobs, snapshot)
	return "", nil
}
