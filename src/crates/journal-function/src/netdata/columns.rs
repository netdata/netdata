//! Column schemas for the logs table of the systemd-journal Functions
//! protocol: the per-column metadata the UI reads from the response's
//! `columns` object - identity, visibility, type, sort, filter and cell
//! formatting (`valueOptions`).
//!
//! `generate_column_schema` builds the schema map - the special
//! `timestamp`/`rowOptions` columns at indexes 0-1, then one column per
//! discovered journal field - from the histogram's discovered fields
//! (`discovered_fields` of `journal-engine/src/histogram.rs`'s
//! `Histogram`); `columns_to_sorted_json`
//! serializes it with keys in index order into the `columns` field of
//! [`crate::netdata::types::JournalResponse`]. `netdata/response.rs`
//! renders the row arrays against the same schema
//! ([`crate::netdata::response::table_to_netdata_response`]). Sole
//! in-tree caller (grep-verified):
//! [`crate::netdata::builder::build_ui_response`], whose output
//! `otel-legacy-logs/src/handler.rs` (`LegacyLogsHandler::on_call`) puts
//! into the response.
//!
//! Outside this crate, otel-ledger's logs adapter hand-builds the same
//! column shape for its tables (`build_columns` in
//! `otel-ledger/src/ledger/rpc/logs/adapter.rs`).

use serde::{Deserialize, Serialize};
use serde_json::{Map, Value};
use std::collections::HashMap as StdHashMap;

/// How the UI can filter a column, serialized as `"facet"`, `"range"` or
/// `"none"` (snake_case).
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum FilterType {
    /// Facet filtering: the UI offers the column's distinct values as filter choices
    Facet,
    /// Range filtering - part of the protocol, but no column sets it today
    Range,
    /// No filtering available
    None,
}

/// Cell-formatting instructions the UI applies to a column's values
/// (see `build_columns` in `otel-ledger/src/ledger/rpc/logs/adapter.rs`).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ValueOptions {
    /// UI-side transform for the cell value: "datetime_usec" renders µs
    /// epoch values as datetimes, "none" passes them through
    pub transform: String,
    /// Number of decimal points for numeric values
    pub decimal_points: u32,
    /// Default value when field is missing
    pub default_value: Option<String>,
}

// Defaults for regular field columns (for_field): no transform, no
// decimals, "-" shown for missing values.
impl Default for ValueOptions {
    fn default() -> Self {
        Self {
            transform: "none".to_string(),
            decimal_points: 0,
            default_value: Some("-".to_string()),
        }
    }
}

/// Schema of one column of the logs table, serialized into the response's
/// `columns` object keyed by `key`.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ColumnSchema {
    /// Position in the table; also drives the JSON key order
    /// (columns_to_sorted_json)
    pub index: usize,

    /// The column's identifier: the map key here, the JSON object key in
    /// [`columns_to_sorted_json`], and the value
    /// [`crate::netdata::response::table_to_netdata_response`] matches on.
    /// Skipped in serialization - `id` mirrors it.
    #[serde(skip)]
    pub key: String,

    /// Column identifier as it appears in the serialized schema
    pub id: String,

    /// Whether this column is a unique key
    pub unique_key: bool,

    /// Display name for the column
    pub name: String,

    /// Whether the column is visible by default
    pub visible: bool,

    /// Data type of the column ("timestamp", "none" or "string" for the
    /// columns built here), serialized as `type`
    #[serde(rename = "type")]
    pub column_type: String,

    /// How the UI renders the column: "value" for data columns,
    /// "rowOptions" for the pseudo-column
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

    /// Whether this is a dummy column (true only for rowOptions); omitted
    /// from the JSON when `None`
    #[serde(skip_serializing_if = "Option::is_none")]
    pub dummy: Option<bool>,
}

impl ColumnSchema {
    /// The special "timestamp" column (index 0): the only `unique_key`
    /// column, a µs epoch value the UI renders through the `datetime_usec`
    /// transform, no filter.
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
            filter: FilterType::None,
            full_width: false,
            wrap: true,
            default_expanded_filter: false,
            dummy: None,
        }
    }

    /// The special "rowOptions" column (index 1): an invisible dummy
    /// column that reserves row position 1, where the rendered rows carry
    /// the per-row options object `{"severity": ...}` that
    /// [`crate::netdata::response::table_to_netdata_response`] emits.
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

    /// Schema for a regular field column: one column per discovered
    /// journal field. Indexes 0-1 belong to the special columns, so
    /// `index` must be >= 2 (debug-asserted).
    pub fn for_field(index: usize, field_name: &str) -> Self {
        debug_assert!(index >= 2);

        let name = field_name.to_string();

        // Filter type mirrors the C implementation: FACET for every field,
        // NONE for the ones registered FACET_KEY_OPTION_NEVER_FACET there
        // (src/libnetdata/facets/facets.c).
        let filter = match name.as_str() {
            // Fields with FACET_KEY_OPTION_NEVER_FACET in C code
            "MESSAGE" | "ND_JOURNAL_PROCESS" | "ND_JOURNAL_FILE" => FilterType::None,
            // All other fields get facet filter by default
            _ => FilterType::Facet,
        };

        // Only MESSAGE is visible by default (timestamp and rowOptions set
        // their own).
        let visible = name == "MESSAGE";

        // Filters shown expanded on first view, mirroring the C plugin's
        // EXPANDED_FILTER registrations (FACET_KEY_OPTION_EXPANDED_FILTER in
        // src/collectors/systemd-journal.plugin/systemd-journal.c).
        let default_expanded_filter =
            matches!(name.as_str(), "PRIORITY" | "SYSLOG_FACILITY" | "MESSAGE_ID");

        // MESSAGE and the OTEL body field (log.body) take the full width.
        let full_width = (name == "MESSAGE") || (name == "log.body");

        // Same field set as full_width: MESSAGE and log.body wrap.
        let wrap = (name == "MESSAGE") || (name == "log.body");

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
            wrap,
            default_expanded_filter,
            dummy: None,
        }
    }
}

/// Builds the full column schema map for the logs table: the special
/// columns at indexes 0-1 (`timestamp`, `rowOptions`) plus one column per
/// discovered field from index 2 on. Filter types come from
/// `ColumnSchema::for_field`. The UI expects the serialized `columns`
/// object's keys in index order, so convert the map through
/// `columns_to_sorted_json`.
///
/// # Arguments
///
/// * `discovered_fields` - Sorted field names from
///   `journal_engine::Histogram::discovered_fields()`
///   (in `journal-engine/src/histogram.rs`), as passed by
///   [`crate::netdata::builder::build_ui_response`]. Names matching the special columns are skipped.
///
/// # Returns
///
/// A map from column key to schema.
pub fn generate_column_schema(discovered_fields: &[String]) -> StdHashMap<String, ColumnSchema> {
    let mut columns = StdHashMap::new();

    // Special columns at indexes 0 and 1.
    let timestamp = ColumnSchema::timestamp();
    columns.insert(timestamp.key.clone(), timestamp);

    let row_options = ColumnSchema::row_options();
    columns.insert(row_options.key.clone(), row_options);

    // Discovered fields from index 2 on.
    let mut index = 2;
    for field_name in discovered_fields.iter() {
        // Colliding names would overwrite the special columns (and shift
        // the field indexes), so skip them.
        if field_name == "timestamp" || field_name == "rowOptions" {
            continue;
        }

        let schema = ColumnSchema::for_field(index, field_name);
        columns.insert(schema.key.clone(), schema);
        index += 1;
    }

    columns
}

/// Serializes the schema map into the response's `columns` object with the
/// keys in index order, as the UI expects. Insertion order survives
/// because journal-function enables serde_json's `preserve_order` feature
/// (`journal-function/Cargo.toml`); without it `serde_json::Map`
/// alphabetizes keys.
pub fn columns_to_sorted_json(columns: &StdHashMap<String, ColumnSchema>) -> Value {
    let mut entries: Vec<_> = columns.iter().collect();
    entries.sort_by_key(|(_, schema)| schema.index);

    let mut map = Map::new();
    for (key, schema) in entries {
        if let Ok(value) = serde_json::to_value(schema) {
            // Insertion order = index order (preserve_order feature).
            map.insert(key.clone(), value);
        }
    }

    Value::Object(map)
}

#[cfg(test)]
mod tests {
    use super::*;

    // Regression guard: the timestamp column's filter used to serialize as
    // "range" and was switched to "none" when its filters were aligned
    // with the systemd-journal SDK; no column may offer "range" again.
    #[test]
    fn serialized_columns_do_not_offer_timestamp_range_filter() {
        let schema = generate_column_schema(&["PRIORITY".to_string()]);
        let columns = columns_to_sorted_json(&schema);
        let columns = columns.as_object().expect("columns should be an object");
        let timestamp = columns
            .get("timestamp")
            .expect("timestamp column should be present");

        assert_eq!(timestamp["filter"], "none");
        for (key, column) in columns {
            assert_ne!(
                column.get("filter").and_then(Value::as_str),
                Some("range"),
                "column {key}"
            );
        }
    }
}
