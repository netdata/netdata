// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	promselector "github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	promcollector "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
	"gopkg.in/yaml.v3"
)

type validationOptions struct {
	profilePath string
	dumpPath    string
	jobPath     string
}

// jobPolicy deliberately exposes only shaping settings. Endpoint, credentials,
// TLS, and profile selection are forced by the validator so a validation run
// cannot contact an arbitrary service or load a different profile.
type jobPolicy struct {
	Name           string                       `yaml:"name,omitempty"`
	Application    string                       `yaml:"app,omitempty"`
	Selector       promselector.Expr            `yaml:"selector,omitempty"`
	Relabeling     []promcollector.RelabelBlock `yaml:"relabeling,omitempty"`
	FallbackType   fallbackTypePolicy           `yaml:"fallback_type,omitempty"`
	ExpectedPrefix string                       `yaml:"expected_prefix,omitempty"`
	MaxTS          *int                         `yaml:"max_time_series,omitempty"`
	MaxTSPerMetric *int                         `yaml:"max_time_series_per_metric,omitempty"`
}

type fallbackTypePolicy struct {
	Gauge   []string `yaml:"gauge,omitempty"`
	Counter []string `yaml:"counter,omitempty"`
}

func loadJobPolicy(path string) (jobPolicy, error) {
	if path == "" {
		return jobPolicy{}, nil
	}
	if err := validateInputFile(path); err != nil {
		return jobPolicy{}, fmt.Errorf("job policy: %w", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return jobPolicy{}, fmt.Errorf("read job policy %q: %w", path, err)
	}

	var policy jobPolicy
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

	if policy.MaxTS != nil && *policy.MaxTS < 0 {
		return jobPolicy{}, fmt.Errorf("job policy: max_time_series must be non-negative")
	}
	if policy.MaxTSPerMetric != nil && *policy.MaxTSPerMetric < 0 {
		return jobPolicy{}, fmt.Errorf("job policy: max_time_series_per_metric must be non-negative")
	}
	return policy, nil
}

func applyJobPolicy(coll *promcollector.Collector, policy jobPolicy, fileURL, profileName string) effectiveJobReport {
	if policy.Name == "" {
		coll.Name = "profile_validation"
	} else {
		coll.Name = policy.Name
	}
	coll.Application = policy.Application
	coll.URL = fileURL
	coll.Selector = policy.Selector
	coll.Relabeling = slices.Clone(policy.Relabeling)
	coll.ExpectedPrefix = policy.ExpectedPrefix
	coll.FallbackType.Gauge = slices.Clone(policy.FallbackType.Gauge)
	coll.FallbackType.Counter = slices.Clone(policy.FallbackType.Counter)
	if policy.MaxTS != nil {
		coll.MaxTS = *policy.MaxTS
	}
	if policy.MaxTSPerMetric != nil {
		coll.MaxTSPerMetric = *policy.MaxTSPerMetric
	}
	coll.Profiles = promcollector.ProfilesConfig{
		Mode: "exact",
		ModeExact: &promcollector.ProfilesModeConfig{
			Entries: []promcollector.ProfileEntryConfig{{Name: profileName}},
		},
	}

	return effectiveJobReport{
		Name:               coll.Name,
		App:                coll.Application,
		SelectorAllow:      slices.Clone(coll.Selector.Allow),
		SelectorDeny:       slices.Clone(coll.Selector.Deny),
		RelabelBlocks:      len(coll.Relabeling),
		FallbackGauge:      slices.Clone(coll.FallbackType.Gauge),
		FallbackCounter:    slices.Clone(coll.FallbackType.Counter),
		ExpectedPrefix:     coll.ExpectedPrefix,
		MaxTimeSeries:      coll.MaxTS,
		MaxSeriesPerMetric: coll.MaxTSPerMetric,
	}
}
