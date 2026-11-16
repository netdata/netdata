//! Assembles the logs-UI table response for the systemd-journal Functions
//! protocol: one call turns the query's `Histogram` run plus the matched
//! `LogEntryData` entries into the `(columns, data)` JSON pair the logs
//! table consumes.
//!
//! Division of labor: field names come from the histogram's discovered
//! fields (`discovered_fields` of `journal-engine/src/histogram.rs`'s
//! `Histogram`, sorted order); the column
//! schema and its index-ordered JSON come from `netdata/columns.rs`
//! ([`crate::netdata::columns::generate_column_schema`] /
//! [`crate::netdata::columns::columns_to_sorted_json`]); the per-entry
//! table transforms every cell through the systemd registry
//! ([`systemd_transformations`], `netdata/transformations.rs`); the final
//! row arrays come from `netdata/response.rs`
//! ([`crate::netdata::response::table_to_netdata_response`]).
//! Cells keep the raw value beside the display string
//! (`CellValue::with_display` in `journal-engine/src/logs/table.rs`), which
//! is what lets the rendered rows carry raw microsecond timestamps while
//! showing formatted values.
//!
//! Sole consumer (grep-verified): `otel-legacy-logs/src/handler.rs`
//! (`LegacyLogsHandler::on_call`), which feeds the pair into the
//! `columns`/`data` fields of [`crate::netdata::types::JournalResponse`]
//! through the `netdata` re-export of [`build_ui_response`].

use crate::netdata::transformations::{TransformationRegistry, systemd_transformations};
use journal_core::Result;
use journal_engine::{CellValue, Histogram, LogEntryData, Table};
use serde_json;
use std::collections::HashMap;
use tracing::warn;

/// Lays the entries out in a table, transforming every cell on the way.
///
/// Same column layout as the engine's plain `entry_data_to_table`
/// (`journal-engine/src/logs/table.rs`) - `timestamp` first, then
/// `column_names` in order - but cells are written through the registry
/// ([`TransformationRegistry::transform_field`]): a field with a
/// registered transform becomes a (raw, display) value pair, everything
/// else keeps display = raw. Entries carry a microsecond timestamp plus
/// field=value pairs (`LogEntryData` in `journal-engine/src/logs/query.rs`); pairs
/// outside `column_names` are dropped.
///
/// The `Result` wrapper is vestigial: nothing here can fail, so it always
/// resolves to `Ok`. `build_ui_response` keeps an `Err` arm as a guard in
/// case table building grows a real failure mode.
fn entry_data_to_table_with_transformations(
    entry_data: &[LogEntryData],
    column_names: Vec<String>,
    transformations: &TransformationRegistry,
) -> Result<Table> {
    let mut all_columns = vec!["timestamp".to_string()];
    all_columns.extend(column_names.clone());
    let mut transformed_table = Table::new(all_columns);

    let column_map: HashMap<&str, usize> = column_names
        .iter()
        .enumerate()
        .map(|(idx, name)| (name.as_str(), idx + 1)) // +1 because timestamp is at index 0
        .collect();

    // Rows are pre-filled so cells can be written by index.
    for data in entry_data {
        let num_cols = column_names.len() + 1;
        let mut row = vec![CellValue::new(None); num_cols];

        // Timestamp column: the "timestamp" key routes it to the µs → RFC3339
        // transform (`SourceRealtimeTimestampTransformation`).
        row[0] = transformations.transform_field("timestamp", Some(data.timestamp.to_string()));

        // Requested fields only: transform in place, skip anything outside
        // column_names.
        for pair in &data.fields {
            if let Some(&col_idx) = column_map.get(pair.field()) {
                row[col_idx] =
                    transformations.transform_field(pair.field(), Some(pair.value().to_string()));
            }
        }

        transformed_table.add_row(row);
    }

    Ok(transformed_table)
}

/// Builds the `(columns, data)` pair for a logs-table response.
///
/// Pipeline:
/// 1. Column names from the histogram's discovered fields, in the
///    histogram's sorted order (`discovered_fields` in
///    `journal-engine/src/histogram.rs`).
/// 2. Column schema generated
///    ([`crate::netdata::columns::generate_column_schema`]) and serialized
///    with keys in index order
///    ([`crate::netdata::columns::columns_to_sorted_json`]).
/// 3. Entries laid into a transformed table - see
///    `entry_data_to_table_with_transformations` above.
/// 4. Table rendered into row arrays
///    ([`crate::netdata::response::table_to_netdata_response`]): each row is
///    `[timestamp_usecs, {severity}, field, ...]`.
///
/// Empty input short-circuits to `({}, [])` - no schema is produced when
/// nothing matched. If table building ever errors, the schema is still
/// returned alongside an empty `data` and a `warn!`; that arm is
/// unreachable today (see the helper's doc).
///
/// # Arguments
///
/// * `histogram` - The histogram run whose discovered fields name the columns
/// * `log_entries` - The matched entries, still in raw field=value form
///
/// # Returns
///
/// `(columns, data)`, ready to drop into the `columns`/`data` fields of
/// [`crate::netdata::types::JournalResponse`]. Sole call site:
/// `otel-legacy-logs/src/handler.rs` (`LegacyLogsHandler::on_call`).
pub fn build_ui_response(
    histogram: &Histogram,
    log_entries: &[LogEntryData],
) -> (serde_json::Value, serde_json::Value) {
    // No entries: skip schema generation entirely.
    if log_entries.is_empty() {
        return (serde_json::json!({}), serde_json::json!([]));
    }

    let field_names: Vec<String> = histogram
        .discovered_fields()
        .iter()
        .map(|f| f.to_string())
        .collect();
    let column_schema = super::columns::generate_column_schema(&field_names);
    let columns = super::columns::columns_to_sorted_json(&column_schema);

    // systemd field transforms: PRIORITY names, RFC3339 timestamps, uids, ...
    let transformations = systemd_transformations();

    match entry_data_to_table_with_transformations(log_entries, field_names, &transformations) {
        Ok(table) => {
            let ui_data_rows = super::response::table_to_netdata_response(&table, &column_schema);
            (columns, serde_json::json!(ui_data_rows))
        }
        // Defensive only - the helper has no error path today; the UI still
        // gets the schema with empty data.
        Err(e) => {
            warn!("failed to create table from log entries: {}", e);
            (columns, serde_json::json!([]))
        }
    }
}
