//! High-level builder for Netdata UI responses.
//!
//! This module provides convenience functions for building complete Netdata UI
//! responses from log data and histogram information.

use crate::netdata::transformations::{TransformationRegistry, systemd_transformations};
use journal_core::Result;
use journal_engine::{CellValue, Histogram, LogEntryData, Table};
use serde_json;
use std::collections::HashMap;
use tracing::{info, warn};

/// Wrapper around entry_data_to_table that applies Netdata transformations.
fn entry_data_to_table_with_transformations(
    entry_data: &[LogEntryData],
    column_names: Vec<String>,
    transformations: &TransformationRegistry,
) -> Result<Table> {
    // Create table with same column structure
    let mut all_columns = vec!["timestamp".to_string()];
    all_columns.extend(column_names.clone());
    let mut transformed_table = Table::new(all_columns);

    // Create a mapping from column name to index for fast lookup
    let column_map: HashMap<&str, usize> = column_names
        .iter()
        .enumerate()
        .map(|(idx, name)| (name.as_str(), idx + 1)) // +1 because timestamp is at index 0
        .collect();

    // Process each entry with transformations
    for data in entry_data {
        let num_cols = column_names.len() + 1;
        let mut row = vec![CellValue::new(None); num_cols];

        // First column: timestamp (transformed)
        row[0] = transformations.transform_field("timestamp", Some(data.timestamp.to_string()));

        // Extract and transform requested fields
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

/// Build a complete Netdata UI response from log entries and histogram data.
///
/// This is a high-level convenience function that:
/// 1. Generates column schema from histogram response
/// 2. Builds a table with the log entries and discovered fields
/// 3. Transforms the table to Netdata UI JSON format
/// 4. Returns both the column schema and data for the UI
///
/// # Arguments
///
/// * `histogram` - The histogram containing discovered fields
/// * `log_entries` - The log entry data to format
///
/// # Returns
///
/// A tuple of `(columns, data)` where:
/// - `columns` is the JSON serialization of the column schema
/// - `data` is the JSON array of formatted log rows
///
/// # Example
///
/// ```ignore
/// use journal_function::netdata::build_ui_response;
///
/// let (columns, data) = build_ui_response(&histogram, &log_entries);
/// ```
pub fn build_ui_response(
    histogram: &Histogram,
    log_entries: &[LogEntryData],
) -> (serde_json::Value, serde_json::Value) {
    if log_entries.is_empty() {
        return (serde_json::json!([]), serde_json::json!([]));
    }

    // Generate column schema from histogram
    let field_names: Vec<String> = histogram
        .discovered_fields()
        .iter()
        .map(|f| f.to_string())
        .collect();
    let column_schema = super::columns::generate_column_schema(&field_names);
    // Convert to JSON with keys sorted by index (required by UI)
    let columns = super::columns::columns_to_sorted_json(&column_schema);

    let transformations = systemd_transformations();

    match entry_data_to_table_with_transformations(log_entries, field_names, &transformations) {
        Ok(table) => {
            info!(
                "table has {} rows and {} columns",
                table.row_count(),
                table.column_count()
            );

            // Transform to UI format
            let ui_data_rows = super::response::table_to_netdata_response(&table, &column_schema);

            info!("transformed to {} UI data rows", ui_data_rows.len());

            (columns, serde_json::json!(ui_data_rows))
        }
        Err(e) => {
            warn!("failed to create table from log entries: {}", e);
            (columns, serde_json::json!([]))
        }
    }
}
