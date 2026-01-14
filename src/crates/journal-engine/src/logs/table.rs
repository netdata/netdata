use super::query::LogEntryData;
use journal_core::Result;
use std::collections::HashMap;
use std::fmt;

/// A cell value with both raw and display representations
#[derive(Debug, Clone)]
pub struct CellValue {
    pub raw: Option<String>,
    pub display: Option<String>,
}

impl CellValue {
    /// Create a new cell value with no transformation
    pub fn new(value: Option<String>) -> Self {
        Self {
            raw: value.clone(),
            display: value,
        }
    }

    /// Create a new cell value with separate raw and display representations
    pub fn with_display(raw: Option<String>, display: Option<String>) -> Self {
        Self { raw, display }
    }
}

/// Column metadata for a table, compatible with the JSON response format
#[derive(Debug, Clone)]
pub struct ColumnInfo {
    pub name: String,
    pub index: usize,
}

impl ColumnInfo {
    pub fn new(name: String, index: usize) -> Self {
        Self { name, index }
    }
}

/// A table representation of log entries with extracted field values
#[derive(Debug, Clone)]
pub struct Table {
    pub columns: Vec<ColumnInfo>,
    pub data: Vec<Vec<CellValue>>,
}

impl Table {
    /// Create a new empty table with the given column names
    pub fn new(column_names: Vec<String>) -> Self {
        let columns = column_names
            .into_iter()
            .enumerate()
            .map(|(index, name)| ColumnInfo::new(name, index))
            .collect();

        Self {
            columns,
            data: Vec::new(),
        }
    }

    /// Add a row to the table
    pub fn add_row(&mut self, row: Vec<CellValue>) {
        self.data.push(row);
    }

    /// Get the number of rows in the table
    pub fn row_count(&self) -> usize {
        self.data.len()
    }

    /// Get the number of columns in the table
    pub fn column_count(&self) -> usize {
        self.columns.len()
    }

    /// Get the column metadata
    pub fn columns(&self) -> &[ColumnInfo] {
        &self.columns
    }

    /// Get the table rows
    pub fn rows(&self) -> &[Vec<CellValue>] {
        &self.data
    }

    /// Calculate the optimal column widths for display
    fn calculate_column_widths(&self) -> Vec<usize> {
        const MESSAGE_MAX_WIDTH: usize = 80;

        let mut widths: Vec<usize> = self.columns.iter().map(|col| col.name.len()).collect();

        // Check each row to find the maximum width needed for each column
        for row in &self.data {
            for (col_idx, cell) in row.iter().enumerate() {
                let display_len = cell.display.as_deref().unwrap_or("-").len();
                if display_len > widths[col_idx] {
                    widths[col_idx] = display_len;
                }
            }
        }

        // Cap the MESSAGE column width at MESSAGE_MAX_WIDTH
        for (col_idx, col) in self.columns.iter().enumerate() {
            if col.name == "MESSAGE" && widths[col_idx] > MESSAGE_MAX_WIDTH {
                widths[col_idx] = MESSAGE_MAX_WIDTH;
            }
        }

        widths
    }
}

impl fmt::Display for Table {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        if self.columns.is_empty() {
            return writeln!(f, "(empty table)");
        }

        let widths = self.calculate_column_widths();
        let total_width: usize = widths.iter().sum::<usize>() + (widths.len() - 1) * 3 + 2;

        // Print top border
        writeln!(f, "{}", "=".repeat(total_width))?;

        // Print header
        write!(f, "|")?;
        for (col, width) in self.columns.iter().zip(&widths) {
            write!(f, " {:<width$} |", col.name, width = width)?;
        }
        writeln!(f)?;

        // Print separator
        writeln!(f, "{}", "=".repeat(total_width))?;

        // Print rows
        for row in &self.data {
            write!(f, "|")?;
            for (cell, width) in row.iter().zip(&widths) {
                let display = cell.display.as_deref().unwrap_or("-");
                // Truncate if the value is longer than the column width
                if display.len() > *width {
                    let truncated = &display[..*width];
                    write!(f, " {:<width$} |", truncated, width = width)?;
                } else {
                    write!(f, " {:<width$} |", display, width = width)?;
                }
            }
            writeln!(f)?;
        }

        // Print bottom border
        writeln!(f, "{}", "=".repeat(total_width))?;

        Ok(())
    }
}

/// Converts extracted entry data into a table with specified columns.
///
/// This function takes raw field data and builds a table structure.
/// It only extracts fields that are in the requested columns list.
///
/// # Arguments
///
/// * `entry_data` - Vector of extracted entry data
/// * `column_names` - Names of fields to include (timestamp is always prepended)
///
/// # Returns
///
/// A `Table` containing the raw field values
pub fn entry_data_to_table(
    entry_data: &[LogEntryData],
    column_names: Vec<String>,
) -> Result<Table> {
    // Always prepend "timestamp" as the first column
    let mut all_columns = vec!["timestamp".to_string()];
    all_columns.extend(column_names.clone());

    let mut table = Table::new(all_columns);

    // Create a mapping from column name to index for fast lookup
    let column_map: HashMap<&str, usize> = column_names
        .iter()
        .enumerate()
        .map(|(idx, name)| (name.as_str(), idx + 1)) // +1 because timestamp is at index 0
        .collect();

    // Process each entry
    for data in entry_data {
        let num_cols = column_names.len() + 1;
        let mut row = vec![CellValue::new(None); num_cols];

        // First column: timestamp
        row[0] = CellValue::new(Some(data.timestamp.to_string()));

        // Extract requested fields
        for pair in &data.fields {
            if let Some(&col_idx) = column_map.get(pair.field()) {
                row[col_idx] = CellValue::new(Some(pair.value().to_string()));
            }
        }

        table.add_row(row);
    }

    Ok(table)
}
