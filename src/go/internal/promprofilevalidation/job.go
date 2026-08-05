// SPDX-License-Identifier: GPL-3.0-or-later

package promprofilevalidation

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	promcollector "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
	"gopkg.in/yaml.v3"
)

// Options identifies one profile, source-complete exposition dump, and
// optional safe job-shaping policy to validate.
type Options struct {
	ProfilePath string
	DumpPath    string
	JobPath     string
}

type jobPolicy struct {
	promcollector.Config `yaml:",inline"`
	FutureInputs         []futureInput `yaml:"future_inputs,omitempty"`
}

var allowedJobPolicyKeys = map[string]struct{}{
	"name": {}, "app": {}, "selector": {}, "relabeling": {},
	"fallback_type": {}, "expected_prefix": {},
	"max_time_series": {}, "max_time_series_per_metric": {},
	"future_inputs": {},
}

func loadJobPolicy(path string) (jobPolicy, error) {
	policy := jobPolicy{Config: promcollector.DefaultConfig()}
	if path == "" {
		return policy, nil
	}
	if err := validateInputFile(path); err != nil {
		return jobPolicy{}, fmt.Errorf("job policy: %w", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return jobPolicy{}, fmt.Errorf("read job policy %q: %w", path, err)
	}

	if err := validateJobPolicyTopLevel(raw); err != nil {
		return jobPolicy{}, fmt.Errorf("decode job policy %q: %w", path, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&policy); err != nil {
		return jobPolicy{}, fmt.Errorf("decode job policy %q: %w", path, err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return jobPolicy{}, fmt.Errorf("decode job policy %q: multiple YAML documents are not allowed", path)
		}
		return jobPolicy{}, fmt.Errorf("decode job policy %q: %w", path, err)
	}

	if policy.MaxTS < 0 {
		return jobPolicy{}, fmt.Errorf("job policy: max_time_series must be non-negative")
	}
	if policy.MaxTSPerMetric < 0 {
		return jobPolicy{}, fmt.Errorf("job policy: max_time_series_per_metric must be non-negative")
	}
	if err := validateFutureInputs(policy.FutureInputs); err != nil {
		return jobPolicy{}, fmt.Errorf("job policy: %w", err)
	}
	return policy, nil
}

func validateJobPolicyTopLevel(raw []byte) error {
	var document yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&document); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple YAML documents are not allowed")
		}
		return err
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return errors.New("job policy must be one YAML mapping")
	}
	root := document.Content[0]
	for i := 0; i < len(root.Content); i += 2 {
		key := root.Content[i]
		if key.Kind != yaml.ScalarNode {
			return fmt.Errorf("job policy key at line %d must be a scalar", key.Line)
		}
		if _, ok := allowedJobPolicyKeys[key.Value]; !ok {
			return fmt.Errorf("field %s not found in capability-limited validation job policy", key.Value)
		}
	}
	return nil
}

func applyJobPolicy(coll *promcollector.Collector, policy jobPolicy, fileURL, profileName string) effectiveJobReport {
	coll.Config = policy.Config
	if coll.Name == "" {
		coll.Name = "profile_validation"
	}
	coll.URL = fileURL
	coll.Profiles = promcollector.ProfilesConfig{
		Mode: "exact",
		ModeExact: &promcollector.ProfilesModeConfig{
			Entries: []promcollector.ProfileEntryConfig{{Name: profileName}},
		},
	}

	return effectiveJobReport{
		Name:                 coll.Name,
		App:                  coll.Application,
		SelectorAllow:        slices.Clone(coll.Selector.Allow),
		SelectorDeny:         slices.Clone(coll.Selector.Deny),
		RelabelBlocks:        len(coll.Relabeling),
		FallbackGauge:        slices.Clone(coll.FallbackType.Gauge),
		FallbackCounter:      slices.Clone(coll.FallbackType.Counter),
		ExpectedPrefix:       coll.ExpectedPrefix,
		MaxTimeSeries:        coll.MaxTS,
		MaxSeriesPerMetric:   coll.MaxTSPerMetric,
		DeclaredFutureInputs: len(policy.FutureInputs),
	}
}
