//! Column schema generation for the logs table UI.
//!
//! This module provides types and functions for generating the column schema
//! that defines how log entries should be displayed in the Netdata dashboard.

use serde::{Deserialize, Serialize};
use serde_json::{Map, Value};
use std::collections::HashMap as StdHashMap;

/// Filter type for a column in the logs table.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum FilterType {
    /// Faceted filtering (for fields with enumerable values)
    Facet,
    /// Range filtering (for numeric/timestamp values)
    Range,
    /// No filtering available
    None,
}

/// Value transformation options for a column.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ValueOptions {
    /// Transformation to apply (e.g., "datetime_usec", "none")
    pub transform: String,
    /// Number of decimal points for numeric values
    pub decimal_points: u32,
    /// Default value when field is missing
    pub default_value: Option<String>,
}

impl Default for ValueOptions {
    fn default() -> Self {
        Self {
            transform: "none".to_string(),
            decimal_points: 0,
            default_value: Some("-".to_string()),
        }
    }
}

/// Complete schema for a single column in the logs table.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ColumnSchema {
    /// Column index (position in the table)
    pub index: usize,

    /// Column identifier/key
    #[serde(skip)]
    pub key: String,

    pub id: String,

    /// Whether this column is a unique key
    pub unique_key: bool,

    /// Display name for the column
    pub name: String,

    /// Whether the column is visible by default
    pub visible: bool,

    /// Data type of the column
    #[serde(rename = "type")]
    pub column_type: String,

    /// How to visualize the column
    pub visualization: String,

    /// Value transformation options
    pub value_options: ValueOptions,

    /// Sort direction
    pub sort: String,

    /// Whether the column is sortable
    pub sortable: bool,

    /// Whether the column is sticky (stays visible when scrolling)
    pub sticky: bool,

    /// Summary function for aggregation
    pub summary: String,

    /// Filter type for this column
    pub filter: FilterType,

    /// Whether the column should take full width
    pub full_width: bool,

    /// Whether text should wrap in the column
    pub wrap: bool,

    /// Whether the filter should be expanded by default
    pub default_expanded_filter: bool,

    /// Whether this is a dummy column (for rowOptions)
    #[serde(skip_serializing_if = "Option::is_none")]
    pub dummy: Option<bool>,
}

impl ColumnSchema {
    /// Create the special "timestamp" column (index 0).
    pub fn timestamp() -> Self {
        Self {
            index: 0,
            id: "timestamp".to_string(),
            key: "timestamp".to_string(),
            unique_key: true,
            name: "Timestamp".to_string(),
            visible: true,
            column_type: "timestamp".to_string(),
            visualization: "value".to_string(),
            value_options: ValueOptions {
                transform: "datetime_usec".to_string(),
                decimal_points: 0,
                default_value: None,
            },
            sort: "ascending".to_string(),
            sortable: false,
            sticky: false,
            summary: "count".to_string(),
            filter: FilterType::Range,
            full_width: false,
            wrap: true,
            default_expanded_filter: false,
            dummy: None,
        }
    }

    /// Create the special "rowOptions" column (index 1).
    pub fn row_options() -> Self {
        Self {
            index: 1,
            id: "rowOptions".to_string(),
            key: "rowOptions".to_string(),
            unique_key: false,
            name: "rowOptions".to_string(),
            visible: false,
            column_type: "none".to_string(),
            visualization: "rowOptions".to_string(),
            value_options: ValueOptions {
                transform: "none".to_string(),
                decimal_points: 0,
                default_value: None,
            },
            sort: "ascending".to_string(),
            sortable: false,
            sticky: false,
            summary: "count".to_string(),
            filter: FilterType::None,
            full_width: false,
            wrap: false,
            default_expanded_filter: false,
            dummy: Some(true),
        }
    }

    /// Create a schema for a regular field column.
    ///
    /// # Arguments
    ///
    /// * `index` - Column index (must be >= 2, as 0 and 1 are special columns)
    /// * `field_name` - The journal field name
    pub fn for_field(index: usize, field_name: &str) -> Self {
        debug_assert!(index >= 2);

        let name = field_name.to_string();

        // Determine filter type - matches C implementation (facets.c:2733)
        // Default is FACET for all fields, EXCEPT those marked NEVER_FACET
        let filter = match name.as_str() {
            // Fields with FACET_KEY_OPTION_NEVER_FACET in C code
            "MESSAGE" | "ND_JOURNAL_PROCESS" | "ND_JOURNAL_FILE" => FilterType::None,
            // All other fields get facet filter by default
            _ => FilterType::Facet,
        };

        // Determine visibility - most fields hidden by default
        // Only MESSAGE is visible by default (timestamp is always visible)
        let visible = name == "MESSAGE";

        // Determine if filter should be expanded by default
        let default_expanded_filter =
            matches!(name.as_str(), "PRIORITY" | "SYSLOG_FACILITY" | "MESSAGE_ID");

        // MESSAGE gets full width
        let full_width = name == "MESSAGE";

        Self {
            index,
            id: name.clone(),
            key: name.clone(),
            unique_key: false,
            name,
            visible,
            column_type: "string".to_string(),
            visualization: "value".to_string(),
            value_options: ValueOptions::default(),
            sort: "ascending".to_string(),
            sortable: false,
            sticky: false,
            summary: "count".to_string(),
            filter,
            full_width,
            wrap: true,
            default_expanded_filter,
            dummy: None,
        }
    }
}

/// Generate the complete column schema map for the logs table UI.
///
/// This function creates the full schema including:
/// 1. Special "timestamp" column (index 0) - with range filter
/// 2. Special "rowOptions" column (index 1) - UI-only, no filter
/// 3. All discovered fields from the histogram (index 2+) - with facet or no filter
///
/// Filter type logic matches C implementation (facets.c:2733):
/// - Default: `filter: "facet"` for all fields
/// - Exception: MESSAGE, ND_JOURNAL_PROCESS, ND_JOURNAL_FILE → `filter: "none"`
///
/// **IMPORTANT**: When serializing to JSON, the keys must be sorted by their `index`
/// field to match the order expected by the UI. Use `columns_to_sorted_json()` helper.
///
/// # Arguments
///
/// * `discovered_fields` - Ordered list of field names from HistogramResponse::discovered_fields()
///
/// # Returns
///
/// A HashMap mapping column keys to their schemas.
pub fn generate_column_schema(discovered_fields: &[String]) -> StdHashMap<String, ColumnSchema> {
    let mut columns = StdHashMap::new();

    // Add special columns first (in index order)
    let timestamp = ColumnSchema::timestamp();
    columns.insert(timestamp.key.clone(), timestamp);

    let row_options = ColumnSchema::row_options();
    columns.insert(row_options.key.clone(), row_options);

    // Add discovered fields starting at index 2 (in index order)
    // Skip special columns (timestamp, rowOptions) if they appear in discovered_fields
    let mut index = 2;
    for field_name in discovered_fields.iter() {
        // Skip special column names to avoid overwriting them
        if field_name == "timestamp" || field_name == "rowOptions" {
            continue;
        }

        let schema = ColumnSchema::for_field(index, field_name);
        columns.insert(schema.key.clone(), schema);
        index += 1;
    }

    columns
}

/// Convert column schema HashMap to JSON with keys sorted by index.
///
/// The UI expects column keys in the JSON object to appear in index order (0, 1, 2, ...).
/// This function sorts the HashMap entries by their `index` field before creating the JSON.
///
/// **IMPORTANT**: This requires serde_json's "preserve_order" feature to be enabled,
/// so that `serde_json::Map` uses IndexMap internally and preserves insertion order.
///
/// # Arguments
///
/// * `columns` - HashMap of column schemas
///
/// # Returns
///
/// A JSON Value with columns as an object where keys are ordered by index.
pub fn columns_to_sorted_json(columns: &StdHashMap<String, ColumnSchema>) -> Value {
    // Collect entries and sort by index
    let mut entries: Vec<_> = columns.iter().collect();
    entries.sort_by_key(|(_, schema)| schema.index);

    // Build JSON object with keys in index order
    // With preserve_order feature, serde_json::Map preserves insertion order
    let mut map = Map::new();
    for (key, schema) in entries {
        if let Ok(value) = serde_json::to_value(schema) {
            map.insert(key.clone(), value);
        }
    }

    Value::Object(map)
}
