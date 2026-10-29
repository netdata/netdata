//! Chart and dimension metadata: the data model the chart machinery publishes.
//!
//! [`ChartMetadata`] holds one chart's definition fields (the `CHART` line), its
//! [`DimensionMetadata`] entries (the `DIMENSION` lines), and the chart and
//! dimension ids referenced by `BEGIN`/`SET` updates. `NetdataChart::chart_metadata()`
//! (annotation extraction in `chart_trait`) builds it from a chart struct's
//! schemars annotations, `TrackedChart` (tracker) stores it per registration,
//! and `ChartWriter` (writer) serializes it to the pluginsd protocol. The chart
//! vocabulary ([`ChartType`], [`DimensionAlgorithm`]) belongs to
//! `netdata-plugin-protocol`, which encodes the lines.

use std::collections::HashMap;

pub use netdata_plugin_protocol::{ChartType, DimensionAlgorithm};

/// Metadata for one dimension: the fields of a `DIMENSION` definition line.
#[derive(Debug, Clone)]
pub struct DimensionMetadata {
    /// Dimension id: written on the `DIMENSION` line and referenced by the
    /// `SET` ids of updates. For derived charts it is the struct field name,
    /// which also matches what `ChartDimensions::write_dimensions` emits.
    pub id: String,
    /// Display name shown in the UI (defaults to the id).
    pub name: String,
    /// Algorithm for processing values.
    pub algorithm: DimensionAlgorithm,
    /// Multiplier applied to the raw value before storage (default 1).
    pub multiplier: i64,
    /// Divisor applied to the raw value before storage (default 1).
    pub divisor: i64,
    /// Whether this dimension is emitted with the `hidden` flag. Schema
    /// extraction drops `x-dimension-hidden` fields before building metadata,
    /// so dimensions extracted from a schema are never hidden; `true` here
    /// only comes from hand-built metadata.
    pub hidden: bool,
}

impl DimensionMetadata {
    /// Build a dimension with `name` set to the id and defaults (`absolute`,
    /// multiplier/divisor 1, not hidden). Annotation extraction starts from
    /// this and applies the `x-dimension-*` overrides.
    pub fn new(id: impl Into<String>) -> Self {
        let id = id.into();
        Self {
            name: id.clone(),
            id,
            algorithm: DimensionAlgorithm::default(),
            multiplier: 1,
            divisor: 1,
            hidden: false,
        }
    }
}

/// Metadata for one chart: the `CHART` definition line and its dimensions.
///
/// Built by `extract_chart_metadata` (via `NetdataChart::chart_metadata()`)
/// from a chart struct's schema annotations, stored on `TrackedChart` and
/// serialized by `ChartWriter::write_chart_definition`.
#[derive(Debug, Clone)]
pub struct ChartMetadata {
    /// Chart id. A template id carries the `{instance}` placeholder
    /// ("cpu.{instance}") which [`ChartMetadata::instantiate`] replaces;
    /// otherwise it is concrete ("cpu.cpu0").
    pub id: String,
    /// Chart name (optional, usually empty)
    pub name: String,
    /// Chart title (shown in UI)
    pub title: String,
    /// Units for the chart
    pub units: String,
    /// Family grouping
    pub family: String,
    /// Context for alerts and API
    pub context: String,
    /// Chart type (line, area, stacked, heatmap)
    pub chart_type: ChartType,
    /// Priority for ordering (lower = higher priority)
    pub priority: i64,
    /// Update interval in seconds. Serialized on the `CHART` line; the
    /// per-update `BEGIN` interval in the registry path comes from the
    /// registration interval instead (`TrackedChart::interval`).
    pub update_every: u64,
    /// Dimensions keyed by id (the field name). Stored in a HashMap, so
    /// `DIMENSION` lines are emitted in unspecified order; updates match
    /// dimensions by id, so the order does not matter.
    pub dimensions: HashMap<String, DimensionMetadata>,
    /// Name of the struct field annotated `x-chart-instance` — the
    /// `InstancedChart` instance identifier — or `None`. Cleared by
    /// [`ChartMetadata::instantiate`].
    pub instance_field: Option<String>,
}

impl ChartMetadata {
    /// Build a chart with id-derived defaults: title and context copy the id,
    /// name and family are empty, units are `"value"`, and the chart is a line
    /// chart at priority 1000 with update_every 1. Annotation extraction
    /// starts from this and applies the `x-chart-*` overrides.
    pub fn new(id: impl Into<String>) -> Self {
        let id = id.into();
        Self {
            title: id.clone(),
            context: id.clone(),
            id,
            name: String::new(),
            units: String::from("value"),
            family: String::new(),
            chart_type: ChartType::default(),
            priority: 1000,
            update_every: 1,
            dimensions: HashMap::new(),
            instance_field: None,
        }
    }

    /// True when this is a template: `instance_field` is set or the id
    /// contains the `{instance}` placeholder.
    pub fn is_template(&self) -> bool {
        self.instance_field.is_some() || self.id.contains("{instance}")
    }

    /// Replace the `{instance}` placeholder with a concrete instance id in the
    /// id, name, title, family and context; everything else is copied
    /// unchanged and `instance_field` is cleared, making the copy concrete.
    /// Called by `TrackedChart::new_instanced` with the initial value's
    /// `instance_id()`.
    pub fn instantiate(&self, instance_id: &str) -> Self {
        ChartMetadata {
            id: self.id.replace("{instance}", instance_id),
            name: self.name.replace("{instance}", instance_id),
            title: self.title.replace("{instance}", instance_id),
            units: self.units.clone(),
            family: self.family.replace("{instance}", instance_id),
            context: self.context.replace("{instance}", instance_id),
            chart_type: self.chart_type,
            priority: self.priority,
            update_every: self.update_every,
            dimensions: self.dimensions.clone(),
            instance_field: None,
        }
    }
}
