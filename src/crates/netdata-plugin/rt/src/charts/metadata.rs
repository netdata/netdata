//! Chart and dimension metadata types.

use std::collections::HashMap;

/// Chart type supported by Netdata
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub enum ChartType {
    #[default]
    Line,
    Area,
    Stacked,
}

impl ChartType {
    pub fn as_str(&self) -> &'static str {
        match self {
            ChartType::Line => "line",
            ChartType::Area => "area",
            ChartType::Stacked => "stacked",
        }
    }
}

/// Dimension algorithm for value processing
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub enum DimensionAlgorithm {
    /// Store the value as-is
    #[default]
    Absolute,
    /// Calculate difference from previous value (for counters)
    Incremental,
    /// Calculate percentage of dimension relative to row total
    PercentageOfAbsoluteRow,
    /// Calculate percentage of dimension relative to incremental row
    PercentageOfIncrementalRow,
}

impl DimensionAlgorithm {
    pub fn as_str(&self) -> &'static str {
        match self {
            DimensionAlgorithm::Absolute => "absolute",
            DimensionAlgorithm::Incremental => "incremental",
            DimensionAlgorithm::PercentageOfAbsoluteRow => "percentage-of-absolute-row",
            DimensionAlgorithm::PercentageOfIncrementalRow => "percentage-of-incremental-row",
        }
    }
}

/// Metadata for a single dimension
#[derive(Debug, Clone)]
pub struct DimensionMetadata {
    /// Dimension ID (used in SET commands)
    pub id: String,
    /// Display name (shown in UI)
    pub name: String,
    /// Algorithm for processing values
    pub algorithm: DimensionAlgorithm,
    /// Multiplier for values (default 1)
    pub multiplier: i64,
    /// Divisor for values (default 1)
    pub divisor: i64,
    /// Whether this dimension is hidden
    pub hidden: bool,
}

impl DimensionMetadata {
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

    /// Emit the DIMENSION command
    pub fn emit(&self) -> String {
        let flags = if self.hidden { "hidden" } else { "" };
        format!(
            "DIMENSION {} '{}' {} {} {} {}\n",
            self.id,
            self.name,
            self.algorithm.as_str(),
            self.multiplier,
            self.divisor,
            flags
        )
    }
}

/// Metadata for a chart
#[derive(Debug, Clone)]
pub struct ChartMetadata {
    /// Chart ID (template: "cpu.{instance}" or concrete: "cpu.cpu0")
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
    /// Chart type (line, area, stacked)
    pub chart_type: ChartType,
    /// Priority for ordering (lower = higher priority)
    pub priority: i64,
    /// Update interval in seconds
    pub update_every: u64,
    /// Dimensions in this chart
    pub dimensions: HashMap<String, DimensionMetadata>,
    /// The field name that serves as the instance identifier (if this is a template)
    pub instance_field: Option<String>,
}

impl ChartMetadata {
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

    /// Check if this is a template chart (contains {instance})
    pub fn is_template(&self) -> bool {
        self.instance_field.is_some() || self.id.contains("{instance}")
    }

    /// Instantiate a template with a concrete instance ID
    pub fn instantiate(&self, instance_id: &str) -> Self {
        ChartMetadata {
            id: self.id.replace("{instance}", instance_id),
            name: self.name.replace("{instance}", instance_id),
            title: self.title.replace("{instance}", instance_id),
            units: self.units.clone(),
            family: self.family.replace("{instance}", instance_id),
            context: self.context.replace("{instance}", instance_id),
            chart_type: self.chart_type.clone(),
            priority: self.priority,
            update_every: self.update_every,
            dimensions: self.dimensions.clone(),
            instance_field: None, // No longer a template after instantiation
        }
    }

    /// Emit the CHART command
    pub fn emit_definition(&self) -> String {
        let mut output = format!(
            "CHART {} '{}' '{}' '{}' '{}' '{}' {} {} {}\n",
            self.id,
            self.name,
            self.title,
            self.units,
            self.family,
            self.context,
            self.chart_type.as_str(),
            self.priority,
            self.update_every
        );

        // Emit dimensions
        for dim in self.dimensions.values() {
            output.push_str(&dim.emit());
        }

        output
    }

    /// Emit BEGIN command
    pub fn emit_begin(&self) -> String {
        format!("BEGIN {}\n", self.id)
    }

    /// Emit SET command for a dimension
    pub fn emit_set(&self, dimension_id: &str, value: i64) -> String {
        format!("SET {} = {}\n", dimension_id, value)
    }

    /// Emit END command
    pub fn emit_end(&self) -> String {
        "END\n".to_string()
    }
}
