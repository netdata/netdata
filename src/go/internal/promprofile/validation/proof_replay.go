// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/input"
	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
	"gopkg.in/yaml.v3"
)

// ReplayProofCase runs every fixture through one persistent production
// validation session and returns one detached proof result per fixture.
func ReplayProofCase(
	ctx context.Context,
	opts prominput.ReplayCase,
) ([]promreplay.Result, error) {
	if ctx == nil {
		return nil, fmt.Errorf("proof replay: context is nil")
	}
	if len(opts.FixturePaths) == 0 {
		return nil, fmt.Errorf("proof replay: no fixtures supplied")
	}
	jobPath, cleanup, err := stageProofJob(opts)
	if err != nil {
		return nil, fmt.Errorf("proof replay: %w", err)
	}
	defer cleanup()

	reports, err := validateSequence(ctx, Options{
		ProfilePath:            opts.ProfilePath,
		SupportingProfilePaths: opts.SupportingProfilePaths,
		JobPath:                jobPath,
	}, opts.FixturePaths, newProofValidationMode(opts.DefaultJobName))
	if err != nil {
		return nil, fmt.Errorf("proof replay: %w", err)
	}
	results := make([]promreplay.Result, 0, len(reports))
	for index, report := range reports {
		var details bytes.Buffer
		if err := WriteText(&details, report); err != nil {
			return nil, fmt.Errorf("proof replay step %d report: %w", index, err)
		}
		results = append(results, promreplay.Result{
			Snapshot: report.ResultSnapshot(),
			Details:  details.String(),
		})
	}
	return results, nil
}

func stageProofJob(opts prominput.ReplayCase) (string, func(), error) {
	job := map[string]any{"name": opts.DefaultJobName}
	if opts.MetadataExample != nil {
		resolved, err := resolveMetadataJob(opts.MetadataPath, *opts.MetadataExample)
		if err != nil {
			return "", nil, err
		}
		job = resolved
	}
	if _, ok := job["app"]; ok {
		return "", nil, fmt.Errorf("metadata job must not declare profile-owned app")
	}
	if _, ok := job["profiles"]; ok {
		return "", nil, fmt.Errorf("metadata job must not declare explicit profiles")
	}
	if len(opts.FutureInputs) != 0 {
		ids := make([]string, 0, len(opts.FutureInputs))
		for id := range opts.FutureInputs {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		inputs := make([]prominput.FutureInput, 0, len(ids))
		for _, id := range ids {
			inputs = append(inputs, opts.FutureInputs[id])
		}
		job["future_inputs"] = inputs
	}
	raw, err := yaml.Marshal(job)
	if err != nil {
		return "", nil, fmt.Errorf("encode derived validation job: %w", err)
	}
	if err := validateJobPolicyTopLevel(raw); err != nil {
		return "", nil, fmt.Errorf("derived validation job: %w", err)
	}
	directory, err := os.MkdirTemp("", "netdata-prometheus-proof-job-")
	if err != nil {
		return "", nil, fmt.Errorf("stage derived validation job: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(directory) }
	path := filepath.Join(directory, "job.yaml")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("stage derived validation job: %w", err)
	}
	return path, cleanup, nil
}

func resolveMetadataJob(path string, locator prominput.MetadataExample) (map[string]any, error) {
	if path == "" {
		return nil, fmt.Errorf("metadata path is empty")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read metadata %q: %w", path, err)
	}
	var document struct {
		Modules []struct {
			Meta struct {
				ID string `yaml:"id"`
			} `yaml:"meta"`
			Setup struct {
				Configuration struct {
					Examples struct {
						List []struct {
							Name   string `yaml:"name"`
							Config string `yaml:"config"`
						} `yaml:"list"`
					} `yaml:"examples"`
				} `yaml:"configuration"`
			} `yaml:"setup"`
		} `yaml:"modules"`
	}
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("decode metadata %q: %w", path, err)
	}
	var configs []string
	for _, module := range document.Modules {
		if module.Meta.ID != locator.IntegrationID {
			continue
		}
		for _, example := range module.Setup.Configuration.Examples.List {
			if example.Name == locator.ExampleName {
				configs = append(configs, example.Config)
			}
		}
	}
	if len(configs) != 1 {
		return nil, fmt.Errorf("metadata locator %s/%s resolved %d examples",
			locator.IntegrationID, locator.ExampleName, len(configs))
	}
	var config struct {
		Jobs []map[string]any `yaml:"jobs"`
	}
	decoder := yaml.NewDecoder(bytes.NewBufferString(configs[0]))
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("decode metadata example %s/%s: %w", locator.IntegrationID, locator.ExampleName, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("metadata example %s/%s contains multiple YAML documents",
				locator.IntegrationID, locator.ExampleName)
		}
		return nil, fmt.Errorf("decode metadata example %s/%s: %w", locator.IntegrationID, locator.ExampleName, err)
	}
	var matched []map[string]any
	for _, candidate := range config.Jobs {
		name, ok := candidate["name"].(string)
		if ok && name == locator.JobName {
			matched = append(matched, candidate)
		}
	}
	if len(matched) != 1 {
		return nil, fmt.Errorf("metadata locator %s/%s/%s resolved %d jobs",
			locator.IntegrationID, locator.ExampleName, locator.JobName, len(matched))
	}
	result := make(map[string]any)
	for key, value := range matched[0] {
		if _, safe := allowedJobPolicyKeys[key]; safe || key == "profiles" {
			result[key] = value
		}
	}
	return result, nil
}
