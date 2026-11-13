//! The OTLP metrics charts as chart-protocol text: a chart's definition
//! (`CHART`, `CLABEL`, `CLABEL_COMMIT`, `DIMENSION`), its data slots
//! (`BEGIN`, `SET`, `END`) and its obsolete mark.
//!
//! The lines are written by `netdata_plugin_protocol`, the owner of the rules
//! that keep a line parseable by the agent; this module only decides what goes
//! into them. Everything here comes from a sender (metric names, descriptions,
//! units, attribute keys and values), so nothing is written verbatim: text
//! fields go through the protocol crate's quoting, and chart.rs makes the ids
//! valid with its `sanitize_*` functions before they come here. A malformed
//! command stops the agent's parser, and the agent then restarts or disables
//! the whole plugin (`src/plugins.d/plugins_d.c`), logs and traces included.
//!
//! These bytes are the ingestor's agent-facing output: chart.rs writes each
//! definition, data slot and obsolete mark as one block, the lib.rs tick loop
//! sends the blocks in chunks as `IngestorResponse::ChartData`, and the
//! otel-plugin supervisor writes each chunk to stdout as it is. Line order
//! here is wire order.
//!
//! Consumers: chart.rs uses everything here; metrics_service.rs only
//! [`ChartType`].

use netdata_plugin_protocol::{
    ChartLine, ChartOptions, DimensionAlgorithm, DimensionLine, DimensionOptions, LabelSource,
    write_begin, write_chart, write_clabel, write_clabel_commit, write_dimension, write_end,
    write_set_empty, write_set_int,
};

/// The chart rendering type. This ingestor emits only `Line` and `Heatmap`:
/// line for every chart except the histogram bucket charts
/// (metrics_service.rs).
pub use netdata_plugin_protocol::ChartType;

/// Fixed-point divisor shared by SET and DIMENSION.
///
/// SET values on the wire are integers, so float values are multiplied by
/// this divisor and truncated to i64; every DIMENSION declares it as its
/// divisor (multiplier 1), and the agent scales back when storing:
/// `value = SET_value * 1 / 1000` (src/database/rrdset-collection.c). This
/// preserves three decimal digits of the original float.
const PRECISION_DIVISOR: i64 = 1000;

/// Dashboard order of every OTLP metrics chart.
const PRIORITY: i32 = 1;

/// A chart definition: one `CHART` line, then the labels and a
/// `CLABEL_COMMIT` when there are labels, then one `DIMENSION` line per entry
/// in `dimensions`, in that order.
///
/// The agent requires this block before the first `BEGIN` for the chart, so
/// chart.rs emits it once per (re)definition and only then streams data
/// slots via [`write_data_slot`].
pub struct ChartDefinition {
    /// A valid chart id (`netdata_plugin_protocol::sanitize_chart_id`).
    pub chart_id: String,
    pub title: String,
    pub units: String,
    pub family: String,
    pub context: String,
    pub chart_type: ChartType,
    pub update_every: u64,
    pub labels: Vec<(String, String)>,
    /// The dimensions' ids, valid ones
    /// (`netdata_plugin_protocol::sanitize_dimension_id`); a dimension's
    /// display name is its id.
    pub dimensions: Vec<String>,
}

impl ChartDefinition {
    /// Write the chart's obsolete mark into `buf`: its `CHART` line with the
    /// `obsolete` option. The agent then stops expecting updates and frees
    /// the chart after its cleanup delay; a later definition revives it.
    pub fn write_obsolete(&self, buf: &mut Vec<u8>) {
        self.write_chart_line(buf, true);
    }

    fn write_chart_line(&self, buf: &mut Vec<u8>, obsolete: bool) {
        // The alternate name stays empty: the agent keys the chart by id
        // alone. `store_first` keeps the first collected point instead of
        // dropping it during interpolation (src/database/rrdset.h).
        write_chart(
            buf,
            &ChartLine {
                id: &self.chart_id,
                name: "",
                title: &self.title,
                units: &self.units,
                family: &self.family,
                context: &self.context,
                chart_type: self.chart_type,
                priority: PRIORITY,
                update_every: self.update_every,
                options: ChartOptions {
                    obsolete,
                    store_first: true,
                    hidden: false,
                },
            },
        );
    }

    /// Write the definition block into `buf`.
    pub fn write(&self, buf: &mut Vec<u8>) {
        self.write_chart_line(buf, false);

        // The commit applies the set: labels not re-sent since the last
        // commit are removed.
        if !self.labels.is_empty() {
            for (key, value) in &self.labels {
                write_clabel(buf, key, value, LabelSource::Auto);
            }
            write_clabel_commit(buf);
        }

        // `absolute` stores each SET value as it is, scaled by multiplier /
        // divisor.
        for id in &self.dimensions {
            write_dimension(
                buf,
                &DimensionLine {
                    id,
                    name: id,
                    algorithm: DimensionAlgorithm::Absolute,
                    multiplier: 1,
                    divisor: PRECISION_DIVISOR,
                    options: DimensionOptions::default(),
                },
            );
        }
    }

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

/// One dimension value in a data slot. `None` renders as `SET <id> =`, which
/// stores no value for that dimension in this update.
#[derive(Debug)]
pub struct DimensionValue {
    /// The dimension's id, as in [`ChartDefinition::dimensions`].
    pub id: String,
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
    buf: &mut Vec<u8>,
    chart_id: &str,
    update_every: u64,
    slot_timestamp: u64,
    dimensions: &[DimensionValue],
) {
    write_begin(buf, chart_id, update_every * 1_000_000);

    for dimension in dimensions {
        let id = &dimension.id;
        match dimension.value {
            Some(value) => write_set_int(buf, id, (value * PRECISION_DIVISOR as f64) as i64),
            None => write_set_empty(buf, id),
        }
    }

    write_end(buf, slot_timestamp + update_every);
}
