// SPDX-License-Identifier: GPL-3.0-or-later

// Package topologyv1 contains producer-side types for netdata.topology.v1
// Function payloads.
package topologyv1

import "time"

const (
	SchemaVersion = "netdata.topology.v1"
	ResponseType  = "topology"
)

type Response struct {
	Status         int      `json:"status"`
	Type           string   `json:"type"`
	HasHistory     bool     `json:"has_history,omitempty"`
	AcceptedParams []string `json:"accepted_params,omitempty"`
	RequiredParams []any    `json:"required_params,omitempty"`
	Help           string   `json:"help,omitempty"`
	UpdateEvery    int      `json:"update_every,omitempty"`
	Expires        int64    `json:"expires,omitempty"`
	Data           Data     `json:"data"`
}

func NewResponse(data Data) Response {
	data.SchemaVersion = SchemaVersion
	return Response{
		Status: 200,
		Type:   ResponseType,
		Data:   data,
	}
}

type Data struct {
	SchemaVersion string         `json:"schema_version"`
	Producer      Producer       `json:"producer"`
	CollectedAt   time.Time      `json:"collected_at"`
	ValidAfter    *time.Time     `json:"valid_after,omitempty"`
	ValidUntil    *time.Time     `json:"valid_until,omitempty"`
	View          *View          `json:"view,omitempty"`
	Dictionaries  Dictionaries   `json:"dictionaries"`
	Types         TypeRegistry   `json:"types"`
	Presentation  *Presentation  `json:"presentation,omitempty"`
	Actors        Table          `json:"actors"`
	Links         Table          `json:"links"`
	Evidence      EvidenceMap    `json:"evidence,omitempty"`
	Tables        *DetailTables  `json:"tables,omitempty"`
	Overlays      *OverlayRefs   `json:"overlays,omitempty"`
	Stats         map[string]any `json:"stats,omitempty"`
	Extensions    map[string]any `json:"extensions,omitempty"`
}

type Producer struct {
	Source       string   `json:"source"`
	Instance     string   `json:"instance"`
	NodeID       string   `json:"node_id,omitempty"`
	MachineGUID  string   `json:"machine_guid,omitempty"`
	AgentVersion string   `json:"agent_version,omitempty"`
	Plugin       string   `json:"plugin,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type View struct {
	ID      string   `json:"id,omitempty"`
	Scope   string   `json:"scope,omitempty"`
	Mode    string   `json:"mode,omitempty"`
	GroupBy []string `json:"group_by,omitempty"`
}

type Dictionaries map[string][]any

type TypeRegistry struct {
	ActorTypes        map[string]ActorType        `json:"actor_types"`
	LinkTypes         map[string]LinkType         `json:"link_types"`
	PortTypes         map[string]PortType         `json:"port_types,omitempty"`
	EvidenceTypes     map[string]EvidenceType     `json:"evidence_types,omitempty"`
	TableTypes        map[string]TableType        `json:"table_types,omitempty"`
	OverlayTemplates  map[string]OverlayTemplate  `json:"overlay_templates,omitempty"`
	AggregationScopes map[string]AggregationScope `json:"aggregation_scopes,omitempty"`
}

type ActorType struct {
	Layer             string             `json:"layer"`
	Identity          []string           `json:"identity"`
	MergeIdentity     []string           `json:"merge_identity,omitempty"`
	ParentIdentity    []string           `json:"parent_identity,omitempty"`
	AggregationScopes []string           `json:"aggregation_scopes,omitempty"`
	Presentation      *ActorPresentation `json:"presentation,omitempty"`
}

type LinkType struct {
	Orientation      string            `json:"orientation"`
	DirectionRole    string            `json:"direction_role"`
	Aggregation      LinkAggregation   `json:"aggregation"`
	EvidenceTypes    []string          `json:"evidence_types,omitempty"`
	OverlayTemplates []string          `json:"overlay_templates,omitempty"`
	Presentation     *LinkPresentation `json:"presentation,omitempty"`
}

type PortType struct {
	Presentation *PortPresentation `json:"presentation,omitempty"`
}

type Presentation struct {
	ProfileVersion string                          `json:"profile_version,omitempty"`
	Selection      *SelectionPresentation          `json:"selection,omitempty"`
	Legend         *PresentationLegend             `json:"legend,omitempty"`
	PortFields     []PresentationField             `json:"port_fields,omitempty"`
	ScaleKeys      map[string]ScaleKeyPresentation `json:"scale_keys,omitempty"`
}

type SelectionPresentation struct {
	ActorClick *ActorClickPresentation `json:"actor_click,omitempty"`
}

type ActorClickPresentation struct {
	Mode            string `json:"mode"`
	PathTable       string `json:"path_table,omitempty"`
	PathOwnerColumn string `json:"path_owner_column,omitempty"`
	PathActorColumn string `json:"path_actor_column,omitempty"`
	PathOrderColumn string `json:"path_order_column,omitempty"`
}

type PresentationLegend struct {
	Actors []LegendEntry `json:"actors,omitempty"`
	Links  []LegendEntry `json:"links,omitempty"`
	Ports  []LegendEntry `json:"ports,omitempty"`
}

type LegendEntry struct {
	Type  string `json:"type"`
	Label string `json:"label,omitempty"`
}

type PresentationField struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type ScaleKeyPresentation struct {
	Label string `json:"label"`
	Unit  string `json:"unit,omitempty"`
}

type ActorPresentation struct {
	Label       string                  `json:"label,omitempty"`
	Role        string                  `json:"role,omitempty"`
	Icon        string                  `json:"icon,omitempty"`
	ColorSlot   string                  `json:"color_slot,omitempty"`
	Opacity     string                  `json:"opacity,omitempty"`
	Border      *BorderPresentation     `json:"border,omitempty"`
	Annotation  *AnnotationPresentation `json:"annotation,omitempty"`
	Size        *ActorSizePresentation  `json:"size,omitempty"`
	LabelPolicy *LabelPolicy            `json:"label_policy,omitempty"`
	Ports       *ActorPortsPresentation `json:"ports,omitempty"`
	Hover       *HoverPresentation      `json:"hover,omitempty"`
}

type LinkPresentation struct {
	Label     string                    `json:"label,omitempty"`
	ColorSlot string                    `json:"color_slot,omitempty"`
	Opacity   string                    `json:"opacity,omitempty"`
	LineStyle string                    `json:"line_style,omitempty"`
	Width     string                    `json:"width,omitempty"`
	Curve     string                    `json:"curve,omitempty"`
	Arrow     string                    `json:"arrow,omitempty"`
	Variable  *LinkVariablePresentation `json:"variable,omitempty"`
	Hover     *HoverPresentation        `json:"hover,omitempty"`
}

type PortPresentation struct {
	Label     string `json:"label,omitempty"`
	ColorSlot string `json:"color_slot,omitempty"`
	Opacity   string `json:"opacity,omitempty"`
}

type BorderPresentation struct {
	Enabled   *bool  `json:"enabled,omitempty"`
	ColorSlot string `json:"color_slot,omitempty"`
	Style     string `json:"style,omitempty"`
}

type AnnotationPresentation struct {
	ColorSlot string `json:"color_slot,omitempty"`
	Style     string `json:"style,omitempty"`
}

type ActorSizePresentation struct {
	Mode         string `json:"mode"`
	MetricColumn string `json:"metric_column,omitempty"`
}

type LabelPolicy struct {
	Columns   []string `json:"columns,omitempty"`
	Fallback  string   `json:"fallback,omitempty"`
	MaxLength int      `json:"max_length,omitempty"`
	Array     string   `json:"array,omitempty"`
}

type ActorPortsPresentation struct {
	ShowBullets bool                     `json:"show_bullets,omitempty"`
	Sources     []PortSourcePresentation `json:"sources,omitempty"`
}

type HoverPresentation struct {
	Fields []PresentationField `json:"fields,omitempty"`
}

type PortSourcePresentation struct {
	Source        string `json:"source"`
	Table         string `json:"table,omitempty"`
	Evidence      string `json:"evidence,omitempty"`
	ActorColumn   string `json:"actor_column"`
	NameColumn    string `json:"name_column"`
	TypeColumn    string `json:"type_column,omitempty"`
	DefaultType   string `json:"default_type,omitempty"`
	StatusColumn  string `json:"status_column,omitempty"`
	ModeColumn    string `json:"mode_column,omitempty"`
	RoleColumn    string `json:"role_column,omitempty"`
	SourcesColumn string `json:"sources_column,omitempty"`
}

type LinkVariablePresentation struct {
	Channel     string `json:"channel"`
	ScaleKey    string `json:"scale_key"`
	ValueColumn string `json:"value_column"`
	Min         string `json:"min,omitempty"`
	Max         string `json:"max,omitempty"`
}

type LinkAggregation struct {
	Direction string            `json:"direction"`
	Evidence  string            `json:"evidence,omitempty"`
	Metrics   map[string]string `json:"metrics,omitempty"`
}

type EvidenceType struct {
	LinkType     string   `json:"link_type"`
	Role         string   `json:"role"`
	Columns      []Column `json:"columns"`
	MatchColumns []string `json:"match_columns,omitempty"`
}

type TableType struct {
	Role           string   `json:"role"`
	Owner          string   `json:"owner"`
	Aggregation    string   `json:"aggregation"`
	SourceEvidence string   `json:"source_evidence,omitempty"`
	Columns        []Column `json:"columns"`
}

type OverlayTemplate struct {
	Provider       string       `json:"provider"`
	Contexts       []string     `json:"contexts,omitempty"`
	Dimensions     []string     `json:"dimensions,omitempty"`
	SelectorParams []string     `json:"selector_params,omitempty"`
	Merge          OverlayMerge `json:"merge"`
}

type OverlayMerge struct {
	Refs   string `json:"refs"`
	Values string `json:"values"`
}

type AggregationScope struct {
	Columns        []string `json:"columns"`
	EvidencePolicy string   `json:"evidence_policy,omitempty"`
}

func Bool(value bool) *bool {
	return &value
}

type EvidenceMap map[string]EvidenceSection

type EvidenceSection struct {
	Type  string `json:"type"`
	Table Table  `json:"table"`
}

type DetailTables struct {
	Actor        map[string]DetailTable `json:"actor,omitempty"`
	Relationship map[string]DetailTable `json:"relationship,omitempty"`
}

type DetailTable struct {
	Type  string `json:"type"`
	Table Table  `json:"table"`
}

type OverlayRefs struct {
	Refs *Table `json:"refs,omitempty"`
}

type Table struct {
	Rows    int              `json:"rows"`
	Columns []Column         `json:"columns"`
	Values  []ColumnEncoding `json:"values"`
}

type Column struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Dictionary  string `json:"dictionary,omitempty"`
	Nullable    bool   `json:"nullable,omitempty"`
	Unit        string `json:"unit,omitempty"`
	Role        string `json:"role,omitempty"`
	Aggregation string `json:"aggregation,omitempty"`
}

type ColumnOption func(*Column)

func NewColumn(id, typ string, opts ...ColumnOption) Column {
	column := Column{ID: id, Type: typ}
	for _, opt := range opts {
		opt(&column)
	}
	return column
}

func WithDictionary(name string) ColumnOption {
	return func(column *Column) {
		column.Dictionary = name
	}
}

func WithNullable() ColumnOption {
	return func(column *Column) {
		column.Nullable = true
	}
}

func WithUnit(unit string) ColumnOption {
	return func(column *Column) {
		column.Unit = unit
	}
}

func WithRole(role string) ColumnOption {
	return func(column *Column) {
		column.Role = role
	}
}

func WithAggregation(rule string) ColumnOption {
	return func(column *Column) {
		column.Aggregation = rule
	}
}

type ColumnEncoding interface {
	isColumnEncoding()
}

type ConstEncoding struct {
	Codec string `json:"codec"`
	Value any    `json:"value"`
}

func (ConstEncoding) isColumnEncoding() {}

func Const(value any) ConstEncoding {
	return ConstEncoding{
		Codec: "const",
		Value: value,
	}
}

type ValuesEncoding struct {
	Codec  string `json:"codec"`
	Values []any  `json:"values"`
}

func (ValuesEncoding) isColumnEncoding() {}

func Values(values ...any) ValuesEncoding {
	return ValuesEncoding{
		Codec:  "values",
		Values: append([]any(nil), values...),
	}
}

type DictEncoding struct {
	Codec   string `json:"codec"`
	Values  []any  `json:"values"`
	Indexes []int  `json:"indexes"`
}

func (DictEncoding) isColumnEncoding() {}

func Dict(values []any, indexes ...int) DictEncoding {
	return DictEncoding{
		Codec:   "dict",
		Values:  append([]any(nil), values...),
		Indexes: append([]int(nil), indexes...),
	}
}

func StringValues(values ...string) []any {
	result := make([]any, len(values))
	for i, value := range values {
		result[i] = value
	}
	return result
}
