//! Mapping between the netdata `otel-logs` wire types and the
//! wire-neutral [`sfsq::logs`] engine.
//!
//! [`to_query`] turns an [`OtelLogsRequest`] into the engine's
//! [`LogsQuery`]; [`to_result`] turns the engine's [`LogsData`] into the
//! [`LogsResult`] wire envelope the cloud-frontend renders. The
//! per-structure converters ([`facet_from_sfst`], [`histogram_from_sfst`],
//! [`available_histograms_from_fields`], [`build_table`]) are pure
//! transformers over `sfst` values — no I/O — so they're exercisable
//! against synthetic inputs.

use std::collections::{BTreeSet, HashMap};

use sfsq::logs::{Anchor, Cursor, LogsData, LogsQuery, LogsQueryBuilder};

use super::wire::{
    ACCEPTED_PARAMS, AnchorParam, AvailableHistogram, Chart, ChartDimensions, ChartPoint,
    ChartResult, ChartView, DataPoint, Facet, FacetOption, Histogram, Items, LogsResult,
    MultiSelection, MultiSelectionOption, OtelLogsRequest, Pagination, RequiredParam,
    STREAM_SELECTION_PARAM, Version,
};
use crate::registry::StreamStat;
use file_registry::ServiceStream;

/// One nanosecond expressed as a millisecond fraction. Histogram bucket
/// timestamps go on the wire in milliseconds (legacy chart contract).
const NS_PER_MS: i64 = 1_000_000;

/// One nanosecond expressed as a second fraction. ChartView `after` /
/// `before` / `update_every` are u32 seconds (legacy chart contract).
const NS_PER_S: i64 = 1_000_000_000;

// ── Request canonicalization (wire request → engine query) ──────────
//
// The frontend sends a loose window and no bucket geometry, so the
// handler canonicalizes here — defaulting the window, picking a "nice"
// bucket width, snapping the window outward, and building the histogram
// grid — before handing a fully-specified [`LogsQuery`] to the engine.
// The engine takes the grid as given; deciding it is this layer's job.

/// Default request window in seconds when the frontend doesn't specify
/// `after`/`before`. Matches the consuming UI's default time range.
const DEFAULT_WINDOW_SECS: u32 = 15 * 60;

/// Aim for at least this many time buckets across the window when picking
/// from [`VALID_BUCKET_WIDTHS_S`]. With the curated widths and a
/// 15-minute window this yields 15-second buckets (60 of them).
const TARGET_BUCKETS: u32 = 60;

/// "Nice" bucket widths in seconds. Ported from the legacy systemd-journal
/// plugin's `calculate_bucket_duration` to keep histograms anchored to
/// wall-clock-friendly intervals (1s, 2s, 5s, 10s, 15s, 30s, 1m, 5m, …).
/// [`bucket_width_for_span_s`] picks the largest entry that produces at
/// least [`TARGET_BUCKETS`] buckets across the span, so chart density is
/// stable as the requested window scales.
const VALID_BUCKET_WIDTHS_S: &[u32] = &[
    1, 2, 5, 10, 15, 30, // seconds
    60, 120, 180, 300, 600, 900, 1800, // minutes
    3600, 7200, 21600, 28800, 43200, // hours
    86400, 172800, 259200, 432000, 604800, 1209600, 2592000, // days
];

impl OtelLogsRequest {
    /// Canonicalize this wire request into the engine's neutral
    /// [`LogsQuery`]: default + bucket-align the window, build the
    /// histogram grid, and map the remaining fields. The empty
    /// `histogram` string becomes `None`; a histogram-click µs timestamp
    /// becomes an [`Anchor::Timestamp`] in ns; a malformed cursor string
    /// is dropped (treated as "no anchor").
    ///
    /// Fails with [`sfst::Error::InvalidPattern`] if the free-text `query`
    /// is not a valid regex — validated once here, at the boundary, so a
    /// bad search is a clean request error rather than a per-file degrade.
    pub fn into_query(self) -> Result<LogsQuery, sfst::Error> {
        let (after, before) = effective_window(self.after, self.before);
        let bucket_width_s = bucket_width_for_span_s(before.saturating_sub(after));
        let (after, before) = align_window(after, before, bucket_width_s);

        // `bucket_width_s` divides `(before - after)` exactly after
        // alignment, so no `div_ceil` is needed.
        let grid = sfst::Grid::new(
            (after as i64) * NS_PER_S,
            (bucket_width_s as i64) * NS_PER_S,
            ((before - after) / bucket_width_s) as usize,
        );

        let anchor = self.anchor.and_then(|a| match a {
            AnchorParam::Cursor(s) => Cursor::decode(&s).map(Anchor::Cursor),
            AnchorParam::TimestampUs(us) => {
                Some(Anchor::Timestamp((us as i64).saturating_mul(1_000)))
            }
        });

        let mut builder = LogsQueryBuilder::new(grid)
            .selections(self.selections)
            .facet_fields(self.facets)
            .direction(self.direction)
            .limit(self.last);
        // Empty histogram / no anchor fall through to the builder's
        // defaults (the engine's default dimension; start at the edge).
        if !self.histogram.is_empty() {
            builder = builder.histogram_field(self.histogram);
        }
        if let Some(anchor) = anchor {
            builder = builder.anchor(anchor);
        }
        // Field-less full-text search: an unanchored regex over `key=value`.
        // Validate once here (compile-and-discard) so a malformed pattern is
        // a clean error; the engine recompiles the source per file.
        if !self.query.is_empty() {
            sfst::compile_query(&self.query)?;
            builder = builder.query(self.query);
        }
        Ok(builder.build())
    }

    /// Remove the reserved stream-selector key ([`STREAM_SELECTION_PARAM`])
    /// from `selections` and decode each pick — a stream's `ns_hash` as
    /// lowercase hex, the option id the response advertises — into a
    /// `u64`. Done **before** [`Self::into_query`] so the engine never
    /// treats `__streams` as a row facet. An unparseable pick is skipped
    /// (a UI-supplied filter should degrade, not fail the whole query);
    /// an absent or empty selection yields an empty vec, which
    /// `file_registry::Query::stream_hashes` reads as "all streams".
    pub fn take_stream_hashes(&mut self) -> Vec<u64> {
        self.selections
            .remove(STREAM_SELECTION_PARAM)
            .unwrap_or_default()
            .iter()
            .filter_map(|s| u64::from_str_radix(s, 16).ok())
            .collect()
    }
}

/// Build the otel-logs `required_params` from a tenant's streams: a single
/// [`MultiSelection`] (`__streams`, "Services") whose options are the
/// streams, each pre-selected so the default view spans all of them. An
/// empty `stats` yields `Vec::new()` — a tenant with no streams advertises
/// no selector.
pub fn stream_required_params(stats: Vec<StreamStat>) -> Vec<RequiredParam> {
    if stats.is_empty() {
        return Vec::new();
    }
    let options = stats
        .into_iter()
        .map(|s| MultiSelectionOption {
            id: format!("{:016x}", s.ns_hash),
            name: stream_display_name(&s.stream),
            pill: humanize_bytes(s.total_size),
            info: stream_info(&s),
            default_selected: true,
        })
        .collect();
    vec![RequiredParam::MultiSelection(MultiSelection {
        id: STREAM_SELECTION_PARAM,
        name: "Services".to_string(),
        help: "Filter logs to specific OpenTelemetry service streams \
               (service.namespace / service.name)."
            .to_string(),
        type_: "multiselect",
        options,
    })]
}

/// Human label for a stream option. An absent `service.namespace` (stored
/// as empty) is shown explicitly so it is not confused with a real one.
fn stream_display_name(s: &ServiceStream) -> String {
    match (s.namespace.is_empty(), s.name.is_empty()) {
        (false, _) => format!("{}/{}", s.namespace, s.name),
        (true, false) => format!("{} • (no namespace)", s.name),
        (true, true) => "(unattributed)".to_string(),
    }
}

/// Secondary line for a stream option: file count and, when known, the
/// log-data time span.
fn stream_info(s: &StreamStat) -> String {
    let files = format!(
        "{} file{}",
        s.file_count,
        if s.file_count == 1 { "" } else { "s" }
    );
    match (s.min_timestamp_s, s.max_timestamp_s) {
        (Some(min), Some(max)) if max >= min => {
            format!("{files} · spans {}", humanize_span_s(max - min))
        }
        _ => files,
    }
}

/// Human byte size for the option pill (e.g. `1.5 MiB`, `820 KiB`,
/// `42 B`) — binary units, one decimal, dependency-free.
fn humanize_bytes(bytes: u64) -> String {
    const UNITS: [&str; 6] = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"];
    if bytes < 1024 {
        return format!("{bytes} B");
    }
    let mut value = bytes as f64;
    let mut unit = 0;
    while value >= 1024.0 && unit < UNITS.len() - 1 {
        value /= 1024.0;
        unit += 1;
    }
    format!("{value:.1} {}", UNITS[unit])
}

/// Coarse human duration for a span in seconds — dependency-free, two
/// significant units (e.g. `2d 3h`, `5m 12s`, `<1s`).
fn humanize_span_s(secs: u32) -> String {
    if secs == 0 {
        return "<1s".to_string();
    }
    let units = [("d", 86_400u32), ("h", 3_600), ("m", 60), ("s", 1)];
    let mut rem = secs;
    let mut parts = Vec::new();
    for (label, size) in units {
        let n = rem / size;
        if n > 0 {
            parts.push(format!("{n}{label}"));
            rem %= size;
        }
        if parts.len() == 2 {
            break;
        }
    }
    parts.join(" ")
}

/// The `[after, before)` window (seconds) a [`sfst::Grid`] covers — the
/// span the handler uses to enumerate overlapping SFST candidates.
pub fn window_secs(grid: &sfst::Grid) -> std::ops::Range<u32> {
    let r = grid.range_ns();
    (r.start / NS_PER_S) as u32..(r.end / NS_PER_S) as u32
}

/// Resolve a request's `[after, before)` to a usable window. Returns the
/// inputs verbatim when they form a valid non-empty range; falls back to
/// the last [`DEFAULT_WINDOW_SECS`] from system time otherwise — the
/// `(0, 0)` "no time bound" form and any inverted / zero-width window.
fn effective_window(after: u32, before: u32) -> (u32, u32) {
    let malformed = (after == 0 && before == 0) || after >= before;
    if !malformed {
        return (after, before);
    }
    let now = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_secs() as u32)
        .unwrap_or(u32::MAX);
    (now.saturating_sub(DEFAULT_WINDOW_SECS), now)
}

/// Pick a "nice" bucket width (seconds) for a span: the largest entry in
/// [`VALID_BUCKET_WIDTHS_S`] producing at least [`TARGET_BUCKETS`]
/// buckets. Falls back to `1` for spans too short to satisfy it.
fn bucket_width_for_span_s(span_s: u32) -> u32 {
    VALID_BUCKET_WIDTHS_S
        .iter()
        .rev()
        .find(|&&w| span_s / w >= TARGET_BUCKETS)
        .copied()
        .unwrap_or(1)
}

/// Round `[after, before)` outward to multiples of `width_s` — `after`
/// floored, `before` ceiled — so the histogram grid anchors to absolute
/// wall-clock boundaries (e.g. 15s buckets snap to `t % 15 == 0`). This
/// keeps the chart x-axis stable across the UI's per-second polling:
/// requests within the same bucket-width slot align to the same grid.
fn align_window(after: u32, before: u32, width_s: u32) -> (u32, u32) {
    let aligned_after = (after / width_s) * width_s;
    let aligned_before = before.div_ceil(width_s) * width_s;
    (aligned_after, aligned_before)
}

// ── Engine result → wire envelope ───────────────────────────────────

/// Shape an engine [`LogsData`] into the wire [`LogsResult`].
/// `max_to_return` echoes the request's `last` into `items.max_to_return`.
pub fn to_result(data: LogsData, max_to_return: usize) -> LogsResult {
    let facets = data
        .facets
        .iter()
        .enumerate()
        .map(|(i, f)| facet_from_sfst(i, f))
        .collect();
    let histogram = histogram_from_sfst(&data.histogram_field, &data.histogram);
    let available_histograms = available_histograms_from_fields(&data.available_fields);
    let facetable = data.facetable();
    let (columns, rows) = build_table(&data.rows, &data.columns, &facetable);
    let matched = data.matched as usize;

    LogsResult {
        progress: 100,
        version: Version::default(),
        accepted_params: ACCEPTED_PARAMS.to_vec(),
        required_params: Vec::new(),
        facets,
        available_histograms,
        histogram,
        columns,
        data: rows,
        default_charts: Vec::new(),
        items: Items {
            evaluated: matched,
            unsampled: 0,
            estimated: matched,
            matched,
            // before ⇒ newer rows exist (UI "scroll up"); after ⇒ older
            // rows exist (UI "scroll down").
            before: data.has_newer as usize,
            after: data.has_older as usize,
            returned: data.rows.len(),
            max_to_return,
        },
        show_ids: false,
        has_history: true,
        status: 200,
        response_type: String::from("table"),
        help: String::from("Query and visualize OpenTelemetry logs."),
        pagination: Pagination::default(),
    }
}

// ── Per-structure converters ────────────────────────────────────────

/// Convert one [`sfst::FacetResult`] into a [`Facet`].
///
/// Option order is preserved from the input — [`sfst::IndexReader::facets`]
/// already surfaces values in FST iteration order, which is lexicographic
/// and stable across runs.
fn facet_from_sfst(order: usize, sfst_facet: &sfst::FacetResult) -> Facet {
    let options = sfst_facet
        .values
        .iter()
        .enumerate()
        .map(|(opt_order, (value, count))| FacetOption {
            id: value.clone(),
            name: value.clone(),
            order: opt_order,
            count: *count as usize,
        })
        .collect();

    Facet {
        id: sfst_facet.field.clone(),
        name: sfst_facet.field.clone(),
        order,
        options,
    }
}

/// Convert an [`sfst::Timeline`] into the UI's [`Histogram`] shape.
///
/// `field` is the histogram dimension field (e.g. `"severity_text"`).
/// The timeline's grid drives `chart.view.after` / `before` /
/// `update_every`; per-bucket dimension counts become flat `[count, 0, 0]`
/// triples on each [`DataPoint`].
///
/// Appends an `"(unset)"` trailer dimension counting per-bucket logs that
/// match the filter but don't carry `field`. Matches the legacy
/// systemd-journal wire shape — `result.labels` ends with `"(unset)"`,
/// each `DataPoint.items` carries an extra trailing triple.
fn histogram_from_sfst(field: &str, timeline: &sfst::Timeline) -> Histogram {
    const UNSET_LABEL: &str = "(unset)";

    let total_dim_count = timeline.dimensions.len() + 1; // value dims + (unset)
    let grid = timeline.grid;
    let bucket_start_ms = (grid.bucket_start_ns / NS_PER_MS).max(0) as u64;
    let bucket_width_ms = (grid.bucket_width_ns / NS_PER_MS).max(1) as u64;

    let range_ns = grid.range_ns();
    let after_s = (range_ns.start / NS_PER_S).max(0) as u32;
    let before_s = (range_ns.end / NS_PER_S).max(0) as u32;
    let update_every_s = (grid.bucket_width_ns / NS_PER_S).max(1) as u32;

    let mut dimension_ids: Vec<String> = timeline.dimensions.clone();
    dimension_ids.push(UNSET_LABEL.to_string());
    let dimension_names: Vec<String> = dimension_ids.clone();
    let dimension_units: Vec<String> =
        std::iter::repeat_n("events".to_string(), total_dim_count).collect();

    // Labels carry a leading "time" entry to match the legacy chart
    // contract: result.labels[0] is the timestamp column header,
    // result.labels[1..] line up with each DataPoint's items —
    // dimension values first, then the trailing "(unset)" entry.
    let mut labels: Vec<String> = Vec::with_capacity(total_dim_count + 1);
    labels.push("time".to_string());
    labels.extend(timeline.dimensions.iter().cloned());
    labels.push(UNSET_LABEL.to_string());

    let data: Vec<DataPoint> = timeline
        .buckets
        .iter()
        .enumerate()
        .map(|(bucket_i, bucket)| {
            let timestamp_ms = bucket_start_ms + (bucket_i as u64) * bucket_width_ms;
            let mut items: Vec<[usize; 3]> =
                bucket.counts.iter().map(|&c| [c as usize, 0, 0]).collect();
            items.push([bucket.unset as usize, 0, 0]);
            DataPoint {
                timestamp_ms,
                items,
            }
        })
        .collect();

    Histogram {
        id: field.to_string(),
        name: field.to_string(),
        chart: Chart {
            view: ChartView {
                title: format!("Events distribution by {field}"),
                after: after_s,
                before: before_s,
                update_every: update_every_s,
                units: String::from("events"),
                chart_type: String::from("stackedBar"),
                dimensions: ChartDimensions {
                    ids: dimension_ids,
                    names: dimension_names,
                    units: dimension_units,
                },
            },
            result: ChartResult {
                labels,
                point: ChartPoint {
                    value: 0,
                    arp: 1,
                    pa: 2,
                },
                data,
            },
        },
    }
}

/// Build the `available_histograms` list from the engine's available
/// (low/mid-card) field set. The engine already excludes high-card
/// fields, so this is a straight enumeration in field order.
fn available_histograms_from_fields(fields: &sfst::FieldTable) -> Vec<AvailableHistogram> {
    fields
        .iter()
        .enumerate()
        .map(|(order, f)| AvailableHistogram {
            id: f.name.clone(),
            name: f.name.clone(),
            order,
        })
        .collect()
}

/// Build the wire `columns` schema and `data` rows from a materialized
/// page.
///
/// Columns: a visible µs `timestamp` and `severity`, a hidden string
/// `cursor` (the `pagination.column` the UI echoes as `anchor`), then one
/// hidden column per attribute field. Fields in `facetable` get
/// `filter: "facet"` so the UI's "+ Add Filter Field" picker offers them;
/// everything else is `"none"`. Each data row is a positional array
/// aligned to the column `index`; absent attributes are `null`.
/// Build the log table's column schema: the three fixed columns
/// (`timestamp`, `severity`, hidden `cursor`) plus one per field. Each is a
/// UI-specific metadata blob; a field present in `facetable` gets
/// `filter: "facet"`, the rest `"none"`.
fn build_columns(fields: &[String], facetable: &BTreeSet<&str>) -> serde_json::Value {
    use serde_json::json;

    let mut columns = serde_json::Map::new();
    // The UI formats the cell from `valueOptions.transform`, not from
    // `type` (which only selects the cell component). Match the legacy
    // journal column: a `timestamp` cell carrying a µs value rendered via
    // the `datetime_usec` transform.
    columns.insert(
        "timestamp".into(),
        json!({ "index": 0, "id": "timestamp", "name": "Timestamp", "type": "timestamp",
                "visible": true, "sortable": false, "filter": "none",
                "valueOptions": { "transform": "datetime_usec", "decimal_points": 0 } }),
    );
    columns.insert(
        "severity".into(),
        json!({ "index": 1, "id": "severity", "name": "Severity",
                "type": "string", "visible": false, "sortable": false, "filter": "none" }),
    );
    columns.insert(
        "cursor".into(),
        json!({ "index": 2, "id": "cursor", "name": "cursor", "type": "string",
                "visible": false, "sortable": false, "filter": "none", "unique_key": true }),
    );
    for (i, name) in fields.iter().enumerate() {
        let filter = if facetable.contains(name.as_str()) {
            "facet"
        } else {
            "none"
        };
        columns.insert(
            name.clone(),
            json!({ "index": 3 + i, "id": name, "name": name, "type": "string",
                    "visible": false, "sortable": false, "filter": filter }),
        );
    }
    serde_json::Value::Object(columns)
}

/// Group a materialized row's `(key, value)` pairs by field name,
/// preserving stream order and skipping exact-duplicate values — a
/// multi-valued field (e.g. a flattened scalar array) legitimately carries
/// several values on one row.
fn group_row_fields(row: &sfst::MaterializedRow) -> HashMap<&str, Vec<&str>> {
    let mut lookup: HashMap<&str, Vec<&str>> = HashMap::new();
    for (k, v) in &row.fields {
        let vals = lookup.entry(k.as_str()).or_default();
        if !vals.contains(&v.as_str()) {
            vals.push(v.as_str());
        }
    }
    lookup
}

/// Build one row's positional cells — `[timestamp_µs, severity, cursor,
/// field_0, …]`, aligned to [`build_columns`]'s schema.
///
/// A generic field cell joins the field's values with `", "` (the wire is
/// one string per column; filters/search run on the index, never on
/// cells). The dedicated severity cell takes the **last** `severity_text`
/// value: the indexer interns the projected top-level LogRecord severity
/// after all attributes, so this picks the real severity even when an
/// attribute is also named `severity_text`.
fn build_row_cells(
    cursor: &Cursor,
    row: &sfst::MaterializedRow,
    fields: &[String],
) -> Vec<serde_json::Value> {
    use serde_json::{Value, json};

    let lookup = group_row_fields(row);
    let cell = |name: &str| match lookup.get(name) {
        Some(vals) if vals.len() == 1 => json!(vals[0]),
        Some(vals) => json!(vals.join(", ")),
        None => Value::Null,
    };
    let severity = match lookup.get("severity_text").and_then(|v| v.last()) {
        Some(v) => json!(v),
        None => Value::Null,
    };
    let mut cells: Vec<Value> = Vec::with_capacity(3 + fields.len());
    cells.push(json!(cursor.timestamp_ns / 1_000)); // ns → µs (JS-safe)
    cells.push(severity);
    cells.push(json!(cursor.encode()));
    cells.extend(fields.iter().map(|f| cell(f)));
    cells
}

/// Build the wire `columns` schema and `data` rows from a materialized
/// page — a thin orchestrator over [`build_columns`] and [`build_row_cells`].
fn build_table(
    rows: &[(Cursor, sfst::MaterializedRow)],
    fields: &[String],
    facetable: &BTreeSet<&str>,
) -> (serde_json::Value, serde_json::Value) {
    let columns = build_columns(fields, facetable);
    let data = rows
        .iter()
        .map(|(cursor, row)| serde_json::Value::Array(build_row_cells(cursor, row, fields)))
        .collect();
    (columns, serde_json::Value::Array(data))
}

#[cfg(test)]
mod tests;
