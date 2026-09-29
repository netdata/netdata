// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/native/nativefunc"
	"gopkg.in/yaml.v2"
)

var identifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
var metricName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.]*$`)
var checkStates = []string{"ok", "warning", "critical", "unknown"}

const checkPrefix = "native.check."

const (
	modeOneshot    = "oneshot"
	modePersistent = "persistent"
)

func checkMetric(id string) string { return checkPrefix + id }

type manifest struct {
	Version      string                  `yaml:"version"`
	Mode         string                  `yaml:"mode"`
	Command      []string                `yaml:"command"`
	Metrics      []metricDefinition      `yaml:"metrics"`
	Checks       []checkDefinition       `yaml:"checks"`
	Charts       string                  `yaml:"charts"`
	Functions    []nativefunc.Definition `yaml:"functions"`
	methods      []funcapi.FunctionConfig
	ConfigSchema string `yaml:"config_schema"`
	config       *packageConfig
	metricByName map[string]metricDefinition
	checkByID    map[string]checkDefinition
}

type metricDefinition struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
	Unit string `yaml:"unit"`
}

type checkDefinition struct {
	ID       string   `yaml:"id"`
	Title    string   `yaml:"title"`
	ByLabels []string `yaml:"by_labels"`
}

func loadManifest(path string, validate func(string) (string, error)) (manifest, *chartengine.TemplateSet, error) {
	var m manifest
	if !filepath.IsAbs(path) {
		return m, nil, fmt.Errorf("manifest must be an absolute path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return m, nil, fmt.Errorf("read manifest: %w", err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.SetStrict(true)
	if err := decoder.Decode(&m); err != nil {
		return m, nil, fmt.Errorf("invalid manifest schema")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return m, nil, fmt.Errorf("manifest must contain one YAML document")
	}
	if m.Version != "v1" {
		return m, nil, fmt.Errorf("unsupported manifest version")
	}
	if m.Mode == "" {
		m.Mode = modeOneshot
	}
	if m.Mode != modeOneshot && m.Mode != modePersistent {
		return m, nil, fmt.Errorf("mode must be oneshot or persistent")
	}
	if len(m.Command) == 0 || m.Command[0] == "" {
		return m, nil, fmt.Errorf("manifest command is required")
	}
	if !filepath.IsAbs(m.Command[0]) {
		m.Command[0] = filepath.Join(filepath.Dir(path), m.Command[0])
	}
	m.Command[0], err = validate(m.Command[0])
	if err != nil {
		return m, nil, fmt.Errorf("validate executable: %w", err)
	}
	m.metricByName = make(map[string]metricDefinition, len(m.Metrics))
	for _, d := range m.Metrics {
		if !metricName.MatchString(d.Name) || strings.HasPrefix(d.Name, "native.") {
			return m, nil, fmt.Errorf("invalid or reserved metric name %q", d.Name)
		}
		if d.Type != "gauge" && d.Type != "counter" {
			return m, nil, fmt.Errorf("metric %q requires gauge or counter type", d.Name)
		}
		if _, ok := m.metricByName[d.Name]; ok {
			return m, nil, fmt.Errorf("duplicate metric %q", d.Name)
		}
		if strings.TrimSpace(d.Unit) == "" {
			return m, nil, fmt.Errorf("metric %q requires a unit", d.Name)
		}
		m.metricByName[d.Name] = d
	}
	m.checkByID = make(map[string]checkDefinition, len(m.Checks))
	for _, d := range m.Checks {
		if !identifier.MatchString(d.ID) || strings.TrimSpace(d.Title) == "" {
			return m, nil, fmt.Errorf("check requires a valid id and title")
		}
		if _, ok := m.checkByID[d.ID]; ok {
			return m, nil, fmt.Errorf("duplicate check %q", d.ID)
		}
		seen := map[string]bool{}
		for _, key := range d.ByLabels {
			if !identifier.MatchString(key) || seen[key] {
				return m, nil, fmt.Errorf("check %q has invalid or duplicate identity labels", d.ID)
			}
			seen[key] = true
		}
		m.checkByID[d.ID] = d
	}
	if len(m.Metrics)+len(m.Checks)+len(m.Functions) == 0 {
		return m, nil, fmt.Errorf("manifest must declare metrics, checks or functions")
	}
	m.methods, err = nativefunc.Declarations(m.Functions)
	if err != nil {
		return m, nil, err
	}
	if m.functionOnly() && m.Charts != "" {
		return m, nil, fmt.Errorf("function-only packages cannot declare charts")
	}
	m.config, err = loadPackageConfig(filepath.Dir(path), m.ConfigSchema)
	if err != nil {
		return m, nil, err
	}
	templates, err := m.chartTemplates(filepath.Dir(path))
	return m, templates, err
}

func (m manifest) functionOnly() bool { return len(m.Metrics)+len(m.Checks) == 0 }

func (m manifest) chartTemplates(dir string) (*chartengine.TemplateSet, error) {
	if m.functionOnly() {
		return nil, nil
	}
	spec := chartengine.TemplateSetSpec{
		FallbackContextNamespace: "native_script",
		Policy: chartengine.EnginePolicy{
			Autogen: &chartengine.AutogenPolicy{
				Enabled:                  true,
				ExpireAfterSuccessCycles: 1,
			},
		},
	}
	if m.Charts != "" {
		path := m.Charts
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read charts: %w", err)
		}
		set, err := chartengine.NewTemplateSetYAML(data)
		if err != nil {
			return nil, fmt.Errorf("invalid chart template")
		}
		spec.Entries, spec.Policy, spec.FallbackContextNamespace = set.Entries(), set.GlobalPolicy(), set.FallbackContextNamespace()
		if len(m.Checks) > 0 && spec.Policy.Selector != nil {
			return nil, fmt.Errorf("global chart selector cannot filter built-in checks; use chart selectors instead")
		}
		for _, entry := range spec.Entries {
			if err := validateChartIDs(entry.Groups); err != nil {
				return nil, err
			}
		}
	}
	var groups []charttpl.Group
	for _, check := range m.Checks {
		var instances *charttpl.Instances
		if len(check.ByLabels) > 0 {
			instances = &charttpl.Instances{
				ByLabels: check.ByLabels,
			}
		}
		groups = append(
			groups,
			charttpl.Group{
				Family:  "Checks",
				Metrics: []string{checkMetric(check.ID)},
				Charts: []charttpl.Chart{{
					ID: "native_check_" + check.ID, Title: check.Title, Context: "check_state", Units: "state",
					Instances: instances, Lifecycle: &charttpl.Lifecycle{
						ExpireAfterCycles: 1,
					},
					Dimensions: []charttpl.Dimension{
						{Selector: checkMetric(check.ID), NameFromLabel: checkMetric(check.ID)},
					},
				}},
			},
		)
	}
	if len(groups) > 0 {
		spec.Entries = append(
			[]chartengine.TemplateEntry{{ID: "native_checks", ContextNamespace: "native_script", Groups: groups}},
			spec.Entries...)
	}
	return chartengine.NewTemplateSet(spec)
}

func validateChartIDs(groups []charttpl.Group) error {
	for _, group := range groups {
		for _, chart := range group.Charts {
			if strings.HasPrefix(chart.ID, "native_check_") {
				return fmt.Errorf("chart id prefix native_check_ is reserved")
			}
		}
		if err := validateChartIDs(group.Groups); err != nil {
			return err
		}
	}
	return nil
}
