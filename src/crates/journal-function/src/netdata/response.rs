//! Netdata response formatting.
//!
//! This module converts generic Table structures into the format expected
//! by the Netdata dashboard UI.

use super::columns::ColumnSchema;
use super::severity::Severity;
use journal_engine::Table;
use serde_json::json;
use std::collections::HashMap;

/// Transform a table to the Netdata UI data format.
///
/// Converts the table to the format expected by the Netdata dashboard:
/// ```json
/// [
///   [timestamp_usec, {severity: "info"}, val1, val2, ...],
///   [timestamp_usec, {severity: "error"}, val1, val2, ...],
///   ...
/// ]
/// ```
///
/// # Arguments
///
/// * `table` - The table to convert
/// * `column_schema` - The column schema, used for ordering and visibility
///
/// # Returns
///
/// A vector of JSON values, where each value is an array representing one row.
///
/// # Format
///
/// Each row array contains:
/// 1. Timestamp (u64, microseconds) - from first column
/// 2. rowOptions object with severity - calculated from PRIORITY field
/// 3. Field values in schema order - one per non-hidden column
///
/// # Example
///
/// ```ignore
/// use journal_function::logs::entry_data_to_table;
/// use journal_function::netdata::table_to_netdata_response;
///
/// let table = entry_data_to_table(&log_entries, columns, &transformations)?;
/// let column_schema = netdata::generate_column_schema(&field_names);
/// let ui_data = table_to_netdata_response(&table, &column_schema);
/// response.data = serde_json::to_value(ui_data)?;
/// ```
pub fn table_to_netdata_response(
    table: &Table,
    column_schema: &HashMap<String, ColumnSchema>,
) -> Vec<serde_json::Value> {
    let mut rows = Vec::with_capacity(table.row_count());

    // Build mapping: table column name → table column index
    let col_map: HashMap<&str, usize> = table
        .columns()
        .iter()
        .map(|col| (col.name.as_str(), col.index))
        .collect();

    // Get timestamp column index (should always be present)
    let timestamp_idx = col_map.get("timestamp").copied().unwrap_or(0);

    // Get PRIORITY column index (optional, for severity calculation)
    let priority_idx = col_map.get("PRIORITY").copied();

    // Get non-special columns from schema in index order
    // Exclude timestamp (index 0), rowOptions (index 1), and hidden columns
    let mut schema_cols: Vec<_> = column_schema
        .values()
        .filter(|col| col.key != "timestamp" && col.key != "rowOptions")
        .filter(|col| {
            // Only include visible columns or those explicitly in the table
            col.visible || col_map.contains_key(col.key.as_str())
        })
        .collect();
    schema_cols.sort_by_key(|col| col.index);

    // Transform each row
    for table_row in table.rows() {
        let mut ui_row = Vec::with_capacity(2 + schema_cols.len());

        // Element 0: timestamp (as u64, microseconds)
        let timestamp = table_row
            .get(timestamp_idx)
            .and_then(|cell| cell.raw.as_ref())
            .and_then(|s| s.parse::<u64>().ok())
            .unwrap_or(0);
        ui_row.push(json!(timestamp));

        // Element 1: rowOptions with severity
        let priority_value = priority_idx
            .and_then(|idx| table_row.get(idx))
            .and_then(|cell| cell.raw.as_deref());
        let severity = Severity::from_priority(priority_value);
        ui_row.push(json!({"severity": severity}));

        // Elements 2+: field values in schema order
        for schema_col in &schema_cols {
            if let Some(&table_idx) = col_map.get(schema_col.key.as_str()) {
                // Use display value from the table
                let value = table_row
                    .get(table_idx)
                    .and_then(|cell| cell.display.clone());
                ui_row.push(json!(value));
            } else {
                // Column is in schema but not in table data (shouldn't happen normally)
                ui_row.push(json!(null));
            }
        }

        rows.push(json!(ui_row));
    }

    rows
}
