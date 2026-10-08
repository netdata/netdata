// Table rendering for query results: the render types shared by
// journal-engine and journal-function - `Table`, `CellValue`, `ColumnInfo` -
// plus `entry_data_to_table`, which arranges the `LogEntryData` vectors
// query.rs' `extract_entry_data` produces into that shape.
// `Table`'s doc below maps the pipeline and its consumers.
use super::query::LogEntryData;
use journal_core::Result;
use std::collections::HashMap;
use std::fmt;

/// One table cell: the field's raw journal value kept beside the string
/// rendered for it.
///
/// `new` fills both with the same string; the transformation registry
/// splits them via `with_display`
/// (`journal-function/src/netdata/transformations.rs` `TransformationRegistry::transform_field`), so `raw` stays
/// verbatim while `display` carries the user-facing form. Consumers read
/// them differently: `table_to_netdata_response` parses the timestamp
/// column's `raw` as u64 microseconds and reads PRIORITY's `raw` for
/// severity, but renders field values from `display`. `None` means the
/// entry has no such field: `Display` below prints "-", the UI
/// conversion emits `null`.
#[derive(Debug, Clone)]
pub struct CellValue {
    /// Verbatim value as read from the journal.
    pub raw: Option<String>,
    /// What gets rendered to users.
    pub display: Option<String>,
}

impl CellValue {
    /// Cell with `raw` and `display` both set to `value` - no transformation.
    pub fn new(value: Option<String>) -> Self {
        Self {
            raw: value.clone(),
            display: value,
        }
    }

    /// Cell with distinct `raw` and `display` - how transformations keep
    /// the verbatim value beside the rendered one.
    pub fn with_display(raw: Option<String>, display: Option<String>) -> Self {
        Self { raw, display }
    }
}

/// One column of a [`Table`]: its name and the position its cells occupy
/// in every row vector.
///
/// `Table::new` assigns `index` from position, so it always matches the
/// cell offset; consumers map names back to indexes through it
/// (`journal-function/src/netdata/response.rs` `table_to_netdata_response`).
#[derive(Debug, Clone)]
pub struct ColumnInfo {
    /// Column name as shown in output.
    pub name: String,
    /// Position of this column; cells sit at this offset in each row.
    pub index: usize,
}

impl ColumnInfo {
    /// Column with the given name at the given position.
    pub fn new(name: String, index: usize) -> Self {
        Self { name, index }
    }
}

/// The file's central type: log entries laid out as named columns of
/// [`CellValue`]s - one [`ColumnInfo`] per column, one vector of cells per
/// row, cell order following the column indexes.
///
/// Pipeline position: `entry_data_to_table` below fills it verbatim from
/// `LogEntryData`; journal-function's
/// `entry_data_to_table_with_transformations`
/// (`journal-function/src/netdata/builder.rs`) rebuilds the same row shape
/// with transformations applied - the path production queries take - and
/// `table_to_netdata_response`
/// (`journal-function/src/netdata/response.rs`)
/// renders a `Table` into the logs UI's JSON rows. All names here are
/// re-exported flat (crate root; `journal-function/src/lib.rs`).
///
/// Nothing enforces the row/column shape: `add_row` appends unchecked and
/// the builders here emit `columns.len()` cells per row. `Display` zips
/// rows against the column widths, so short rows render short; an
/// oversized row panics before the zip - `calculate_column_widths` walks
/// every cell against the per-column `widths` vector and indexes past its
/// end. The `Display` impl at the bottom of the file is a human-readable
/// ASCII dump; nothing in-repo renders through it.
#[derive(Debug, Clone)]
pub struct Table {
    /// Column headers; `index` in each matches the cell offset in rows.
    pub columns: Vec<ColumnInfo>,
    /// One vector of cells per entry, in output order.
    pub data: Vec<Vec<CellValue>>,
}

impl Table {
    /// Empty table with one column per name, indexed by position.
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

    /// Appends one row (expected to hold one cell per column).
    pub fn add_row(&mut self, row: Vec<CellValue>) {
        self.data.push(row);
    }

    /// Number of rows.
    pub fn row_count(&self) -> usize {
        self.data.len()
    }

    /// Number of columns.
    pub fn column_count(&self) -> usize {
        self.columns.len()
    }

    /// Column metadata.
    pub fn columns(&self) -> &[ColumnInfo] {
        &self.columns
    }

    /// All rows, in insertion order.
    pub fn rows(&self) -> &[Vec<CellValue>] {
        &self.data
    }

    /// Width per column for `Display`: the larger of the column name's
    /// length and its longest cell display string ("-" when `None` counts
    /// as 1), with the MESSAGE column capped at `MESSAGE_MAX_WIDTH`;
    /// `Display` truncates longer display strings to this width.
    fn calculate_column_widths(&self) -> Vec<usize> {
        const MESSAGE_MAX_WIDTH: usize = 80;

        let mut widths: Vec<usize> = self.columns.iter().map(|col| col.name.len()).collect();

        for row in &self.data {
            for (col_idx, cell) in row.iter().enumerate() {
                let display_len = cell.display.as_deref().unwrap_or("-").len();
                if display_len > widths[col_idx] {
                    widths[col_idx] = display_len;
                }
            }
        }

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
        // Only column-less tables take this branch; a table with columns
        // but no rows still prints the header and borders.
        if self.columns.is_empty() {
            return writeln!(f, "(empty table)");
        }

        let widths = self.calculate_column_widths();
        // Border length: sum of widths plus 3 per gutter - lands 2 chars
        // shorter than the header and row lines below (each cell prints
        // " {value} |").
        let total_width: usize = widths.iter().sum::<usize>() + (widths.len() - 1) * 3 + 2;

        writeln!(f, "{}", "=".repeat(total_width))?;

        write!(f, "|")?;
        for (col, width) in self.columns.iter().zip(&widths) {
            write!(f, " {:<width$} |", col.name, width = width)?;
        }
        writeln!(f)?;

        writeln!(f, "{}", "=".repeat(total_width))?;

        for row in &self.data {
            write!(f, "|")?;
            for (cell, width) in row.iter().zip(&widths) {
                let display = cell.display.as_deref().unwrap_or("-");
                // Truncate to the column width. Byte slicing: a cut that
                // lands inside a multi-byte UTF-8 char panics.
                if display.len() > *width {
                    let truncated = &display[..*width];
                    write!(f, " {:<width$} |", truncated, width = width)?;
                } else {
                    write!(f, " {:<width$} |", display, width = width)?;
                }
            }
            writeln!(f)?;
        }

        writeln!(f, "{}", "=".repeat(total_width))?;

        Ok(())
    }
}

/// Builds a [`Table`] from extracted entry data: one column per requested
/// field, plus a leading `timestamp` column at index 0 holding the entry
/// time as a decimal-microseconds string - exactly what
/// `table_to_netdata_response` parses back as u64
/// (`journal-function/src/netdata/response.rs`).
///
/// Cells are verbatim (`CellValue::new`: `raw` == `display`); fields
/// outside `column_names` are dropped, fields an entry lacks stay `None`,
/// and rows keep `entry_data`'s order. The body never fails - the
/// `Result` is journal-core's alias, kept for the extraction path this
/// mirrors (`logs/query.rs` `extract_entry_data`) - and nothing in-repo calls this function:
/// production renders through journal-function's transformation wrapper
/// (`journal-function/src/netdata/builder.rs`
/// `entry_data_to_table_with_transformations`).
pub fn entry_data_to_table(
    entry_data: &[LogEntryData],
    column_names: Vec<String>,
) -> Result<Table> {
    let mut all_columns = vec!["timestamp".to_string()];
    all_columns.extend(column_names.clone());

    let mut table = Table::new(all_columns);

    let column_map: HashMap<&str, usize> = column_names
        .iter()
        .enumerate()
        .map(|(idx, name)| (name.as_str(), idx + 1)) // +1 because timestamp is at index 0
        .collect();

    for data in entry_data {
        let num_cols = column_names.len() + 1;
        let mut row = vec![CellValue::new(None); num_cols];

        row[0] = CellValue::new(Some(data.timestamp.to_string()));

        for pair in &data.fields {
            if let Some(&col_idx) = column_map.get(pair.field()) {
                row[col_idx] = CellValue::new(Some(pair.value().to_string()));
            }
        }

        table.add_row(row);
    }

    Ok(table)
}
