// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"errors"
	"regexp"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/native/internal/configform"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/native/nativefunc"
)

const (
	formatJSON  = "json"
	formatLines = "lines"

	modeOneshot    = "oneshot"
	modePersistent = "persistent"

	metricGauge    = "gauge"
	metricCounter  = "counter"
	metricStateSet = "stateset"

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

// packageSpec holds static capabilities shared by manifest files and descriptions.
type packageSpec struct {
	SnapshotFormat string                  `yaml:"snapshot_format" json:"snapshot_format"`
	Version        string                  `yaml:"version"         json:"version"`
	Mode           string                  `yaml:"mode"            json:"mode"`
	Collect        *bool                   `yaml:"collect"         json:"collect"`
	Functions      []nativefunc.Definition `yaml:"functions"       json:"functions"`
}

// packageDefinition is a validated package, independent of its source. It is
// immutable after loading; jobs of a registered package share one definition.
type packageDefinition struct {
	packageSpec
	command   []string
	methods   []funcapi.FunctionConfig
	form      *configform.Form // nil when the package declares no configuration
	templates *chartengine.TemplateSet
}

// newPackageDefinition validates capabilities. Sources then
// attach their configuration form and chart templates.
func newPackageDefinition(spec packageSpec, command []string) (packageDefinition, error) {
	d := packageDefinition{
		packageSpec: spec,
		command:     command,
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
	if d.SnapshotFormat == "" {
		d.SnapshotFormat = formatJSON
	}
	if d.SnapshotFormat != formatJSON && d.SnapshotFormat != formatLines {
		return d, errors.New("snapshot_format must be json or lines")
	}
	if d.functionOnly() && d.SnapshotFormat == formatLines {
		return d, errors.New("snapshot_format lines requires collection")
	}
	if d.functionOnly() && len(d.Functions) == 0 {
		return d, errors.New("collect: false requires functions")
	}
	var err error
	d.methods, err = nativefunc.Methods(d.Functions)
	return d, err
}

// functionOnly reports a package without periodic collection or charts.
func (d packageDefinition) functionOnly() bool { return d.Collect != nil && !*d.Collect }
