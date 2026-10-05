// SPDX-License-Identifier: GPL-3.0-or-later

package control

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/filelogreceiver"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/receiver"
)

const Prefix = "otel-poc:"

var validName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type JobConfig struct {
	Paths       []string `json:"paths,omitempty"`
	Interval    string   `json:"interval,omitempty"`
	ServiceName string   `json:"service_name"`
}

type Job struct {
	Kind    string
	Name    string
	Config  JobConfig
	Enabled bool
	Decided bool
	Status  string
}

func (j *Job) ID() string { return Prefix + j.Kind + ":" + j.Name }

func parseConfig(kind string, payload []byte) (JobConfig, error) {
	cfg := JobConfig{ServiceName: "otel-facade-poc"}
	if kind == "hostmetrics" {
		cfg.Interval = "2s"
	}
	if len(bytes.TrimSpace(payload)) == 0 || bytes.TrimSpace(payload)[0] != '{' {
		return cfg, errors.New("configuration must be a JSON object")
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	// The UI includes the job-header name in form data. Identity comes from
	// the DynCfg command, so this metadata must not enter the receiver config.
	form := struct {
		JobConfig
		Name string `json:"name"`
	}{JobConfig: cfg}
	if err := dec.Decode(&form); err != nil {
		return cfg, errors.New("invalid JSON configuration or unknown field")
	}
	cfg = form.JobConfig
	if err := dec.Decode(new(any)); err != io.EOF {
		return cfg, errors.New("configuration must contain one JSON object")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return cfg, err
	}
	for key, value := range fields {
		allowed := key == "name" || key == "service_name" || kind == "filelogs" && key == "paths" || kind == "hostmetrics" && key == "interval"
		if !allowed || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return cfg, errors.New("field is not supported by this job type or is null")
		}
	}
	if cfg.ServiceName == "" {
		return cfg, errors.New("service_name must not be empty")
	}
	switch kind {
	case "filelogs":
		if len(cfg.Paths) == 0 || cfg.Interval != "" {
			return cfg, errors.New("filelogs requires paths and does not accept interval")
		}
		for _, path := range cfg.Paths {
			if !filepath.IsAbs(path) {
				return cfg, errors.New("paths must be absolute file paths or glob patterns")
			}
		}
	case "hostmetrics":
		interval, err := time.ParseDuration(cfg.Interval)
		if err != nil || interval < time.Second || len(cfg.Paths) != 0 {
			return cfg, errors.New("hostmetrics requires interval >= 1s and does not accept paths")
		}
	default:
		return cfg, errors.New("unknown job type")
	}
	// Validate through the real upstream receiver, without starting it.
	factory, raw := receiverConfig(kind, cfg)
	componentConfig := factory.CreateDefaultConfig()
	if err := confmap.NewFromStringMap(raw).Unmarshal(componentConfig); err != nil {
		return cfg, fmt.Errorf("receiver configuration: %w", err)
	}
	if err := confmap.Validate(componentConfig); err != nil {
		return cfg, fmt.Errorf("receiver validation: %w", err)
	}
	return cfg, nil
}

func receiverConfig(kind string, cfg JobConfig) (receiver.Factory, map[string]any) {
	if kind == "filelogs" {
		paths := make([]any, len(cfg.Paths))
		for i, path := range cfg.Paths {
			paths[i] = path
		}
		return filelogreceiver.NewFactory(), map[string]any{
			"include": paths, "start_at": "end", "storage": "file_storage",
		}
	}
	return hostmetricsreceiver.NewFactory(), map[string]any{
		"collection_interval": cfg.Interval,
		"scrapers":            map[string]any{"cpu": map[string]any{}, "memory": map[string]any{}},
	}
}

// configLocked generates only the curated pipelines, never interpolated YAML.
func (c *Controller) configLocked() map[string]any {
	receivers := map[string]any{}
	processors := map[string]any{}
	pipelines := map[string]any{}
	for _, id := range c.jobIDsLocked() {
		job := c.jobs[id]
		if !job.Enabled {
			continue
		}
		factory, receiverCfg := receiverConfig(job.Kind, job.Config)
		name := job.Kind + "_" + job.Name
		rid := factory.Type().String() + "/" + name
		pid := "resource/" + name
		signal := "metrics"
		if job.Kind == "filelogs" {
			signal = "logs"
		}
		receivers[rid] = receiverCfg
		processors[pid] = map[string]any{"attributes": []any{
			map[string]any{"key": "service.name", "value": job.Config.ServiceName, "action": "upsert"},
			map[string]any{"key": "netdata.poc.job", "value": id, "action": "upsert"},
		}}
		pipelines[signal+"/"+name] = map[string]any{
			"receivers": []any{rid}, "processors": []any{pid}, "exporters": []any{"otlp_grpc/netdata"},
		}
	}
	return map[string]any{
		"receivers": receivers, "processors": processors,
		"exporters": map[string]any{"otlp_grpc/netdata": map[string]any{
			"endpoint": c.endpoint, "tls": map[string]any{"insecure": true},
		}},
		"extensions": map[string]any{
			"netdata":      map[string]any{"revision": c.revision},
			"file_storage": map[string]any{"directory": c.storage, "create_directory": true},
		},
		"service": map[string]any{
			"extensions": []any{"netdata", "file_storage"}, "pipelines": pipelines,
			"telemetry": map[string]any{"metrics": map[string]any{"level": "none"}},
		},
	}
}

func (c *Controller) jobIDsLocked() []string {
	ids := make([]string, 0, len(c.jobs))
	for id := range c.jobs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func schema(kind string) map[string]any {
	properties := map[string]any{
		"name": map[string]any{
			"type": "string", "title": "Job name", "description": "Job name entered in the configuration header.",
		},
		"service_name": map[string]any{
			"type": "string", "title": "Service name", "description": "Service name attached to the collected telemetry.",
			"default": "otel-facade-poc", "minLength": 1,
		},
	}
	required := []string{"service_name"}
	if kind == "filelogs" {
		properties["paths"] = map[string]any{
			"type": "array", "title": "Log files", "description": "Absolute file paths or glob patterns. New jobs read new lines; existing offsets survive reloads.",
			"minItems": 1, "items": map[string]any{"type": "string", "title": "Path", "description": "Absolute path or glob pattern."},
		}
		required = append(required, "paths")
	} else {
		properties["interval"] = map[string]any{
			"type": "string", "title": "Collection interval", "description": "Time between CPU and memory collections; at least 1s.", "default": "2s",
		}
		required = append(required, "interval")
	}
	return map[string]any{
		"jsonSchema": map[string]any{"$schema": "http://json-schema.org/draft-07/schema#", "type": "object", "title": "OpenTelemetry " + kind, "properties": properties, "required": required, "additionalProperties": false},
		"uiSchema":   map[string]any{"name": map[string]any{"ui:widget": "hidden"}},
	}
}
