// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/native/internal/configform"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/native/nativefunc"
)

const (
	modeOneshot    = "oneshot"
	modePersistent = "persistent"

	metricGauge   = "gauge"
	metricCounter = "counter"

	// The native namespace is reserved for built-in check series.
	reservedMetricPrefix = "native."
	checkMetricPrefix    = "native.check."
)

var (
	reIdentifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	reMetricName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.]*$`)
)

var checkStates = []string{"ok", "warning", "critical", "unknown"}

func checkMetricName(id string) string { return checkMetricPrefix + id }

// packageSpec holds the declarations shared by manifest files and descriptions.
type packageSpec struct {
	Version   string                  `yaml:"version"   json:"version"`
	Mode      string                  `yaml:"mode"      json:"mode"`
	Metrics   []metricDefinition      `yaml:"metrics"   json:"metrics"`
	Checks    []checkDefinition       `yaml:"checks"    json:"checks"`
	Functions []nativefunc.Definition `yaml:"functions" json:"functions"`
}

type metricDefinition struct {
	Name string `yaml:"name" json:"name"`
	Type string `yaml:"type" json:"type"`
	Unit string `yaml:"unit" json:"unit"`
}

type checkDefinition struct {
	ID       string   `yaml:"id"        json:"id"`
	Title    string   `yaml:"title"     json:"title"`
	ByLabels []string `yaml:"by_labels" json:"by_labels"`
}

// packageDefinition is a validated package, independent of its source. It is
// immutable after loading; jobs of a registered package share one definition.
type packageDefinition struct {
	packageSpec
	command      []string
	methods      []funcapi.FunctionConfig
	form         *configform.Form // nil when the package declares no configuration
	templates    *chartengine.TemplateSet
	metricByName map[string]metricDefinition
	checkByID    map[string]checkDefinition
}

// newPackageDefinition validates declarations and indexes them. Sources then
// attach their configuration form and chart templates.
func newPackageDefinition(spec packageSpec, command []string) (packageDefinition, error) {
	d := packageDefinition{
		packageSpec:  spec,
		command:      command,
		metricByName: make(map[string]metricDefinition, len(spec.Metrics)),
		checkByID:    make(map[string]checkDefinition, len(spec.Checks)),
	}
	if d.Version != "v1" {
		return d, errors.New("unsupported package version")
	}
	if d.Mode == "" {
		d.Mode = modeOneshot
	}
	if d.Mode != modeOneshot && d.Mode != modePersistent {
		return d, errors.New("mode must be oneshot or persistent")
	}
	for _, m := range d.Metrics {
		if !reMetricName.MatchString(m.Name) || strings.HasPrefix(m.Name, reservedMetricPrefix) {
			return d, fmt.Errorf("invalid or reserved metric name %q", m.Name)
		}
		if m.Type != metricGauge && m.Type != metricCounter {
			return d, fmt.Errorf("metric %q requires gauge or counter type", m.Name)
		}
		if _, ok := d.metricByName[m.Name]; ok {
			return d, fmt.Errorf("duplicate metric %q", m.Name)
		}
		if strings.TrimSpace(m.Unit) == "" {
			return d, fmt.Errorf("metric %q requires a unit", m.Name)
		}
		d.metricByName[m.Name] = m
	}
	for _, check := range d.Checks {
		if !reIdentifier.MatchString(check.ID) || strings.TrimSpace(check.Title) == "" {
			return d, errors.New("check requires a valid id and title")
		}
		if _, ok := d.checkByID[check.ID]; ok {
			return d, fmt.Errorf("duplicate check %q", check.ID)
		}
		seen := map[string]bool{}
		for _, key := range check.ByLabels {
			if !reIdentifier.MatchString(key) || seen[key] {
				return d, fmt.Errorf("check %q has invalid or duplicate identity labels", check.ID)
			}
			seen[key] = true
		}
		d.checkByID[check.ID] = check
	}
	if len(d.Metrics)+len(d.Checks)+len(d.Functions) == 0 {
		return d, errors.New("package must declare metrics, checks or functions")
	}
	var err error
	d.methods, err = nativefunc.Methods(d.Functions)
	return d, err
}

// functionOnly reports a package without periodic collection or charts.
func (d packageDefinition) functionOnly() bool { return len(d.Metrics)+len(d.Checks) == 0 }
