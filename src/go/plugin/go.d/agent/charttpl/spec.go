// SPDX-License-Identifier: GPL-3.0-or-later

package charttpl

// VersionV1 is the supported chart-template schema version.
const VersionV1 = "v1"

// Spec is the user-facing chart template file.
type Spec struct {
	Version          string  `yaml:"version" json:"version"`
	ContextNamespace string  `yaml:"context_namespace,omitempty" json:"context_namespace,omitempty"`
	Groups           []Group `yaml:"groups" json:"groups"`
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
	ID               string   `yaml:"id,omitempty" json:"id,omitempty"`
	Title            string   `yaml:"title" json:"title"`
	Family           string   `yaml:"family,omitempty" json:"family,omitempty"`
	Context          string   `yaml:"context" json:"context"`
	Units            string   `yaml:"units" json:"units"`
	Algorithm        string   `yaml:"algorithm,omitempty" json:"algorithm,omitempty"`
	Type             string   `yaml:"type,omitempty" json:"type,omitempty"`
	Priority         int      `yaml:"priority,omitempty" json:"priority,omitempty"`
	LabelPromoted    []string `yaml:"label_promotion,omitempty" json:"label_promotion,omitempty"`

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
	Selector      string `yaml:"selector" json:"selector"`
	Name          string `yaml:"name,omitempty" json:"name,omitempty"`
	NameFromLabel string `yaml:"name_from_label,omitempty" json:"name_from_label,omitempty"`
	Hidden        bool   `yaml:"hidden,omitempty" json:"hidden,omitempty"`
}
