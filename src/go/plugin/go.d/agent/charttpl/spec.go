// SPDX-License-Identifier: GPL-3.0-or-later

package charttpl

import metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"

// VersionV1 is the supported chart-template schema version.
const VersionV1 = "v1"

// Spec is the user-facing chart template file.
type Spec struct {
	Version          string  `yaml:"version" json:"version"`
	ContextNamespace string  `yaml:"context_namespace,omitempty" json:"context_namespace,omitempty"`
	Engine           *Engine `yaml:"engine,omitempty" json:"engine,omitempty"`
	Groups           []Group `yaml:"groups" json:"groups"`
}

// Engine declares template-level chartengine policy.
type Engine struct {
	Selector *metrixselector.Expr `yaml:"selector,omitempty" json:"selector,omitempty"`
	Autogen  *EngineAutogen       `yaml:"autogen,omitempty" json:"autogen,omitempty"`
}

// EngineAutogen controls unmatched-series fallback behavior.
type EngineAutogen struct {
	Enabled bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`

	// TypeID is the chart-type prefix used by Netdata runtime checks
	// (`type.id` length guard). Typically this is `<plugin>.<job>`.
	TypeID string `yaml:"type_id,omitempty" json:"type_id,omitempty"`
	// MaxTypeIDLen is the max allowed full `type.id` length.
	// Zero means default (1200).
	MaxTypeIDLen int `yaml:"max_type_id_len,omitempty" json:"max_type_id_len,omitempty"`
	// ExpireAfterSuccessCycles controls autogen chart/dimension expiry on
	// successful collection cycles where the series is not seen.
	// Zero disables expiry.
	ExpireAfterSuccessCycles uint64 `yaml:"expire_after_success_cycles,omitempty" json:"expire_after_success_cycles,omitempty"`
}

// Group is a recursive chart-group container.
type Group struct {
	Family           string   `yaml:"family" json:"family"`
	ContextNamespace string   `yaml:"context_namespace,omitempty" json:"context_namespace,omitempty"`
	Metrics          []string `yaml:"metrics,omitempty" json:"metrics,omitempty"`

	Groups []Group `yaml:"groups,omitempty" json:"groups,omitempty"`
	Charts []Chart `yaml:"charts,omitempty" json:"charts,omitempty"`
}

// Chart describes one chart template in compact DSL form.
type Chart struct {
	ID            string   `yaml:"id,omitempty" json:"id,omitempty"`
	Title         string   `yaml:"title" json:"title"`
	Family        string   `yaml:"family,omitempty" json:"family,omitempty"`
	Context       string   `yaml:"context" json:"context"`
	Units         string   `yaml:"units" json:"units"`
	Algorithm     string   `yaml:"algorithm,omitempty" json:"algorithm,omitempty"`
	Type          string   `yaml:"type,omitempty" json:"type,omitempty"`
	Priority      int      `yaml:"priority,omitempty" json:"priority,omitempty"`
	LabelPromoted []string `yaml:"label_promotion,omitempty" json:"label_promotion,omitempty"`

	Instances *Instances `yaml:"instances,omitempty" json:"instances,omitempty"`

	Lifecycle *Lifecycle `yaml:"lifecycle,omitempty" json:"lifecycle,omitempty"`

	Dimensions []Dimension `yaml:"dimensions" json:"dimensions"`
}

// Instances defines chart instance identity labels.
type Instances struct {
	ByLabels []string `yaml:"by_labels" json:"by_labels"`
}

// Lifecycle controls chart and dimension cardinality/expiry limits.
type Lifecycle struct {
	// MaxInstances is best-effort per chart template.
	// Active instances in the current successful cycle are not evicted.
	MaxInstances      int                 `yaml:"max_instances,omitempty" json:"max_instances,omitempty"`
	ExpireAfterCycles int                 `yaml:"expire_after_cycles,omitempty" json:"expire_after_cycles,omitempty"`
	Dimensions        *DimensionLifecycle `yaml:"dimensions,omitempty" json:"dimensions,omitempty"`
}

// DimensionLifecycle controls materialized dimension lifecycle limits.
type DimensionLifecycle struct {
	MaxDims           int `yaml:"max_dims,omitempty" json:"max_dims,omitempty"`
	ExpireAfterCycles int `yaml:"expire_after_cycles,omitempty" json:"expire_after_cycles,omitempty"`
}

// Dimension describes one dimension template.
type Dimension struct {
	Selector      string            `yaml:"selector" json:"selector"`
	Name          string            `yaml:"name,omitempty" json:"name,omitempty"`
	NameFromLabel string            `yaml:"name_from_label,omitempty" json:"name_from_label,omitempty"`
	Options       *DimensionOptions `yaml:"options,omitempty" json:"options,omitempty"`
}

// DimensionOptions controls emitted DIMENSION options.
type DimensionOptions struct {
	Multiplier int  `yaml:"multiplier,omitempty" json:"multiplier,omitempty"`
	Divisor    int  `yaml:"divisor,omitempty" json:"divisor,omitempty"`
	Hidden     bool `yaml:"hidden,omitempty" json:"hidden,omitempty"`
}
