//! The logs-UI wire types: the flat serde shapes the netdata builders emit
//! and [`crate::netdata::types::JournalResponse`] carries as `facets`,
//! `available_histograms`, and
//! `histogram` (flat re-exports in `netdata/mod.rs`).
//!
//! This file only defines the shapes - the builders own the semantics:
//! netdata/facets.rs builds `Vec<Facet>` (raw ids, registry-transformed
//! option names, filtered counts), netdata/histogram.rs builds the chart
//! tree and the available-histogram list. Beyond serde, the only live logic
//! is the `count()` chain and the `DataPoint` serde impls below.
//!
//! Consumers (grep-verified): the two builder modules above and
//! `netdata/types.rs` (via its `use super::ui_types as ui`). The response
//! reaches the logs dashboard by serialization; the only code call is
//! `Items.matched` = `Histogram::count()` in
//! `LegacyLogsHandler::on_call` (`otel-legacy-logs/src/handler.rs`).
//!
//! Same-named but independent copies exist in the otel-logs pipeline
//! (`otel-ledger/src/ledger/rpc/logs/wire.rs`); they do not consume
//! this file.

use serde::{Deserialize, Serialize};

// Plain serde structs (Debug + Serialize + Deserialize) except DataPoint,
// which derives only Debug and is flattened to an array by the custom impls
// below. `count()` chains Histogram -> Chart -> ChartResult.

/// One available-histogram option: `id`/`name` are the field name
/// (identical), `order` the 0-based position after sorting by id; one per
/// field with indexed values in the buckets
/// ([`crate::netdata::available_histograms`]).
#[derive(Debug, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct AvailableHistogram {
    pub id: String,
    pub name: String,
    pub order: usize,
}

/// One facet filter for the logs UI: a field with its filterable values.
/// `id`/`name` are the raw field name (identical, not registry-transformed),
/// `order` the field's alphabetical position, `options` its values sorted
/// alphabetically (by [`crate::netdata::facets::facets`]).
#[derive(Debug, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct Facet {
    pub id: String,
    pub name: String,
    pub order: usize,
    pub options: Vec<FacetOption>,
}

/// One filterable value of a `Facet`: `id` the raw value, `name` the
/// registry-transformed display value, `order` the alphabetical position,
/// `count` the post-filter total summed across buckets. Values the active
/// selections exclude still appear, with count 0 (as built by `facets()`
/// in `netdata/facets.rs`;
/// the zero entries are pre-created by `HistogramEngine::compute_from_indexes`
/// in `journal-engine/src/histogram.rs`).
#[derive(Debug, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct FacetOption {
    pub id: String,
    pub name: String,
    pub order: usize,
    pub count: usize,
}

/// The UI histogram chart for one field: `id`/`name` are the field name
/// (identical), `chart` the rendered chart (built by the `histogram()` fn
/// in `netdata/histogram.rs`).
#[derive(Debug, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct Histogram {
    pub id: String,
    pub name: String,
    pub chart: Chart,
}

impl Histogram {
    /// Total events the chart represents; feeds `Items.matched`
    /// (`LegacyLogsHandler::on_call` in `otel-legacy-logs/src/handler.rs`).
    pub fn count(&self) -> usize {
        self.chart.count()
    }
}

/// The rendered chart: `view` the display metadata, `result` the data; both
/// filled by `chart_from_histogram` (`netdata/histogram.rs`).
#[derive(Debug, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct Chart {
    pub view: ChartView,
    pub result: ChartResult,
}

impl Chart {
    /// Delegates to `ChartResult::count`.
    pub fn count(&self) -> usize {
        self.result.count()
    }
}

/// Display metadata, filled with fixed strings by `chart_view_from_histogram`
/// (`netdata/histogram.rs`): title "Events distribution by {field}",
/// `after`/`before` the histogram's time span (first bucket start / last
/// bucket end) in seconds, `update_every` the bucket width in seconds,
/// `units` the literal "units" (the per-dimension units below are
/// "events"), `chart_type` "stackedBar".
#[derive(Debug, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct ChartView {
    pub title: String,
    pub after: u32,
    pub before: u32,
    pub update_every: u32,
    pub units: String,
    pub chart_type: String,
    pub dimensions: ChartDimensions,
}

/// Parallel per-dimension arrays, aligned with `ChartResult`: `ids` the raw
/// field values ("(unset)" last), `names` the display labels
/// (`ChartResult.labels[1..]`), `units` "events" for every dimension
/// (built by `chart_view_from_histogram` in `netdata/histogram.rs`).
#[derive(Debug, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct ChartDimensions {
    pub ids: Vec<String>,
    pub names: Vec<String>,
    pub units: Vec<String>,
}

/// The chart's data. `labels` is ["time", transformed values..., "(unset)"]:
/// `labels[i + 1]` pairs with `ChartDimensions.ids[i]` and
/// `DataPoint.items[i]`; `point` names the item-triple indexes; `data` is
/// one `DataPoint` per bucket, in bucket order (`chart_result_from_histogram`
/// in `netdata/histogram.rs`).
#[derive(Debug, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct ChartResult {
    pub labels: Vec<String>,
    pub point: ChartPoint,
    pub data: Vec<DataPoint>,
}

impl ChartResult {
    /// Total events the chart represents: the value element (`point.value`,
    /// index 0) of every item in every data point, "(unset)" included.
    pub fn count(&self) -> usize {
        let mut n = 0;

        for dp in self.data.iter() {
            for item in dp.items.iter() {
                n += item[0];
            }
        }

        n
    }
}

/// Indexes into each item's [value, arp, pa] triple: 0 = event count,
/// 1 = anomaly rate, 2 = point annotation. The chart builder only ever
/// writes [count, 0, 0] (`chart_result_from_histogram` in
/// `netdata/histogram.rs`); the field meanings
/// come from the former C implementation
/// (`src/libnetdata/facets/facets.c` histogram point emit:
/// `arp` anomaly rate, `pa` point annotation).
#[derive(Debug, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct ChartPoint {
    pub value: u64,
    pub arp: u64,
    pub pa: u64,
}

/// One histogram bucket: `timestamp` the bucket start in ms, `items[i]` the
/// triple for dimension `i` (`chart_result_from_histogram` in
/// `netdata/histogram.rs`). Serde flattens
/// the struct to a plain array - see the impls below.
#[derive(Debug)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct DataPoint {
    pub timestamp: u64,
    pub items: Vec<[usize; 3]>,
}

/// Serializes as a flat array - [timestamp, [value, arp, pa], ...] - rather
/// than a {timestamp, items} object, matching the chart wire format the
/// former C implementation wrote (the histogram `data` emit in
/// `src/libnetdata/facets/facets.c`).
///
/// The deserializer below accepts the same shape.
impl Serialize for DataPoint {
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeSeq;

        // Pre-sized: one timestamp element + one triple per item
        let mut seq = serializer.serialize_seq(Some(1 + self.items.len()))?;

        // Timestamp first
        seq.serialize_element(&self.timestamp)?;

        // Then one [value, arp, pa] triple per item
        for item in &self.items {
            seq.serialize_element(item)?;
        }

        seq.end()
    }
}

impl<'de> Deserialize<'de> for DataPoint {
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        use serde::de::{SeqAccess, Visitor};

        struct DataPointVisitor;

        impl<'de> Visitor<'de> for DataPointVisitor {
            type Value = DataPoint;

            fn expecting(&self, formatter: &mut std::fmt::Formatter) -> std::fmt::Result {
                formatter.write_str("an array with timestamp followed by data items")
            }

            fn visit_seq<A>(self, mut seq: A) -> std::result::Result<Self::Value, A::Error>
            where
                A: SeqAccess<'de>,
            {
                // Timestamp first
                let timestamp = seq
                    .next_element()?
                    .ok_or_else(|| serde::de::Error::invalid_length(0, &self))?;

                // Remaining elements: one [value, arp, pa] triple per item
                let mut items = Vec::new();
                while let Some(item) = seq.next_element()? {
                    items.push(item);
                }

                Ok(DataPoint { timestamp, items })
            }
        }

        deserializer.deserialize_seq(DataPointVisitor)
    }
}
