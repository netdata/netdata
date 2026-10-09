//! Plugin-protocol output formatting for the OTel ingestor: renders chart
//! definitions and data slots as the line-based chart commands the agent
//! parses (`src/plugins.d/pluginsd_parser.c`: CHART, CLABEL, CLABEL_COMMIT,
//! DIMENSION, BEGIN, SET, END).
//!
//! These bytes are the ingestor's agent-facing output: chart.rs renders them
//! into a String buffer, the lib.rs tick loop hands the buffer to the main
//! loop, and the main loop forwards it as `IngestorResponse::ChartData` over
//! ferryboat IPC; the otel-plugin supervisor then writes it raw to stdout
//! (`netdata-plugin/protocol/src/transport.rs` `write_raw` bypasses the IPC
//! framing). Line order here is wire order, every line must be
//! newline-terminated, and a malformed command stops the agent's parser: the
//! plugin is disabled outright if it never collected data, and otherwise
//! restarted with backoff until it is disabled (src/plugins.d/plugins_d.c).
//!
//! Consumers: chart.rs uses everything here but `PRECISION_DIVISOR`;
//! metrics_service.rs only [`ChartType`].

use std::fmt::{self, Write as _};

/// The chart rendering type, written into the CHART line's chart-type field.
///
/// The agent recognizes line, area, stacked and heatmap; this ingestor emits
/// only line and heatmap — line for every chart except the histogram bucket
/// charts, which are heatmap (metrics_service.rs). `fmt::Display` writes
/// the agent's exact keywords.
#[derive(Debug, Clone, Copy, Default)]
pub enum ChartType {
    #[default]
    Line,
    Heatmap,
}

impl fmt::Display for ChartType {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            ChartType::Line => f.write_str("line"),
            ChartType::Heatmap => f.write_str("heatmap"),
        }
    }
}

/// Fixed-point divisor shared by SET and DIMENSION.
///
/// SET values on the wire are integers, so float values are multiplied by
/// this divisor and truncated to i64; every DIMENSION declares it as its
/// divisor (multiplier 1), and the agent scales back when storing:
/// `value = SET_value * 1 / 1000` (src/database/rrdset-collection.c). This
/// preserves three decimal digits of the original float.
pub const PRECISION_DIVISOR: i64 = 1000;

/// Quotes a string for use in a single-quoted CHART/CLABEL field.
///
/// The agent splits each line into words treating `'` and `"` as field
/// delimiters, with only incomplete escape support
/// (src/libnetdata/line_splitter/line_splitter.h), and commands are
/// newline-delimited: an embedded newline would split one command in two,
/// and a malformed command stops the agent's parser
/// (src/plugins.d/pluginsd_internals.c — the module header records the
/// disable policy). Nothing can be escaped, so `'` is
/// rewritten as `"` and `\n`/`\r` as their literal two-character forms.
struct SanitizedQuote<'a>(&'a str);

impl fmt::Display for SanitizedQuote<'_> {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        for ch in self.0.chars() {
            match ch {
                '\'' => f.write_char('"')?,
                '\n' => f.write_str("\\n")?,
                '\r' => f.write_str("\\r")?,
                _ => f.write_char(ch)?,
            }
        }
        Ok(())
    }
}

/// A chart definition: renders as one CHART line, then optional CLABEL
/// lines + CLABEL_COMMIT, then one DIMENSION line per entry in
/// `dimensions`, in that order.
///
/// The agent requires this block before the first BEGIN for the chart, so
/// chart.rs emits it once per (re)definition and only then streams data
/// slots via [`write_data_slot`].
pub struct ChartDefinition {
    pub chart_name: String,
    pub title: String,
    pub units: String,
    pub family: String,
    pub context: String,
    pub chart_type: ChartType,
    pub update_every: u64,
    pub labels: Vec<(String, String)>,
    pub dimensions: Vec<String>,
}

impl fmt::Display for ChartDefinition {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        // CHART <id> <name> <title> <units> <family> <context> <type>
        // <priority> <update_every> <options>: alternate name empty (the
        // agent keys the chart by id alone), priority fixed at 1, and
        // `store_first` keeps the first collected point instead of dropping
        // it during interpolation (src/database/rrdset.h).
        writeln!(
            f,
            "CHART {} '' '{}' '{}' '{}' '{}' {} 1 {} 'store_first'",
            self.chart_name,
            SanitizedQuote(&self.title),
            SanitizedQuote(&self.units),
            SanitizedQuote(&self.family),
            SanitizedQuote(&self.context),
            self.chart_type,
            self.update_every,
        )?;

        // CLABEL <key> <value> <source>, with source 1 = RRDLABEL_SRC_AUTO
        // (src/database/rrdlabels.h). The commit applies the set: labels not
        // re-sent since the last commit are removed.
        if !self.labels.is_empty() {
            for (key, value) in &self.labels {
                writeln!(
                    f,
                    "CLABEL '{}' '{}' 1",
                    SanitizedQuote(key),
                    SanitizedQuote(value),
                )?;
            }
            writeln!(f, "CLABEL_COMMIT")?;
        }

        // DIMENSION <id> <name> <algorithm> <multiplier> <divisor>: id and
        // name are the same string, the one SET lines reference. `absolute`
        // stores each SET value as-is, scaled by multiplier / divisor — the
        // divisor is [`PRECISION_DIVISOR`].
        for dim_name in &self.dimensions {
            writeln!(
                f,
                "DIMENSION {} {} absolute 1 {}",
                dim_name, dim_name, PRECISION_DIVISOR,
            )?;
        }

        Ok(())
    }
}

impl ChartDefinition {
    /// Sort `dimensions` numerically ascending, with `+Inf` last.
    ///
    /// Names that fail to parse are also treated as `+Inf`. Used for heatmap
    /// charts: the agent preserves DIMENSION order, so this is what puts
    /// histogram buckets in ascending order on the dashboard. chart.rs sorts
    /// before emitting a heatmap definition.
    pub fn sort_dimensions_numerically(&mut self) {
        self.dimensions.sort_by(|a, b| {
            let a_val = if a == "+Inf" {
                f64::INFINITY
            } else {
                a.parse::<f64>().unwrap_or(f64::INFINITY)
            };
            let b_val = if b == "+Inf" {
                f64::INFINITY
            } else {
                b.parse::<f64>().unwrap_or(f64::INFINITY)
            };
            a_val
                .partial_cmp(&b_val)
                .unwrap_or(std::cmp::Ordering::Equal)
        });
    }
}

/// One dimension value in a data slot. `None` renders as `SET <name> =`,
/// which stores no value for that dimension in this update.
#[derive(Debug)]
pub struct DimensionValue {
    pub name: String,
    pub value: Option<f64>,
}

/// Write one data slot: BEGIN, one SET per dimension in slice order, END.
///
/// `update_every` is the collection interval in seconds; `slot_timestamp`
/// is the slot's start boundary, already floored to `update_every` by
/// chart.rs.
///
/// BEGIN carries the interval in microseconds: the agent uses it as the
/// time since the previous update. END carries `slot_timestamp +
/// update_every` — the slot's end boundary — because the agent timestamps
/// the point at END: a point at time T covers `[T - update_every, T]`.
pub fn write_data_slot(
    f: &mut impl fmt::Write,
    chart_name: &str,
    update_every: u64,
    slot_timestamp: u64,
    dimensions: &[DimensionValue],
) -> fmt::Result {
    writeln!(f, "BEGIN {} {}", chart_name, update_every * 1_000_000)?;

    for dim in dimensions {
        match dim.value {
            Some(v) => {
                let scaled = (v * PRECISION_DIVISOR as f64) as i64;
                writeln!(f, "SET {} = {}", dim.name, scaled)?;
            }
            None => writeln!(f, "SET {} =", dim.name)?,
        }
    }

    writeln!(f, "END {}", slot_timestamp + update_every)?;
    Ok(())
}
