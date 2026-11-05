//! The store: entities, the open-slot window, the entity cap, expiry and
//! emitting.
//!
//! The store keeps every slot from `next_emit` (the oldest slot not yet
//! emitted) to the current one open; each entity holds those slots as cell
//! arrays and folds them forward after a stall (`advance_window`). Entities
//! are created on their first record, with or without measurements, charts
//! are defined on an entity's first emitted slot, and an expired entity's state is dropped
//! (its cumulative baselines with it).

use crate::batch::Batch;
use crate::metric_set::{Labels, MetricSet, MetricSetInner, Value};
use netdata_plugin_protocol::{
    ChartLine, ChartOptions, DimensionAlgorithm, DimensionLine, DimensionOptions, LabelSource,
    write_begin, write_chart, write_clabel, write_clabel_commit, write_dimension, write_end_timed,
    write_set_empty, write_set_float,
};
use std::collections::{HashMap, VecDeque};

/// Most slots past the lateness one [`Store::emit`] emits after a stall or a
/// forward clock step; older pending slots are folded into the oldest one
/// emitted, so no record is lost and memory stays bounded.
pub(crate) const MAX_BACKLOG_SLOTS: u64 = 60;

/// Counts since the store was created; `entities` is the current number.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct Stats {
    /// Entities currently tracked, the overflow entity included.
    pub entities: usize,
    /// Records counted in the overflow entity because of the cap.
    pub overflowed_records: u64,
    /// Records older than the oldest open slot, counted in that slot.
    pub late_records: u64,
    /// Records newer than the current slot, counted in the current slot.
    pub future_records: u64,
    /// Entities whose charts were marked obsolete after the expiry time.
    pub expired_entities: u64,
}

struct Entity {
    labels: Labels,
    /// The full id of each of the set's charts, in set order.
    chart_ids: Vec<String>,
    defined: bool,
    /// Earliest slot with records; nothing is emitted before it.
    first_slot: u64,
    /// Latest slot with records; drives expiry.
    last_slot: u64,
    /// The cells of each open slot from `next_emit` onwards, oldest first;
    /// missing ones are empty.
    open_cells: VecDeque<Box<[f64]>>,
    /// The last emitted total of each cumulative cell, in set order; NaN
    /// until the first one.
    previous_totals: Box<[f64]>,
}

/// Collects the batches of one [`MetricSet`] and turns finished slots into
/// chart-protocol text.
///
/// The store reads no clock and does no I/O. Pass the current Unix time to
/// [`Store::merge`] and [`Store::emit`], call `emit` at least once per slot,
/// and write the bytes it returns to the agent. Share it behind a lock.
///
/// # When a slot is emitted
///
/// The *current* slot is the one `now` falls in. A slot stays *open* for
/// records until its [`Settings::lateness`](crate::Settings::lateness) has
/// passed after its end; then the next `emit` emits it, once.
///
/// With a 1 s collection interval and 10 s lateness, the slot `[100, 101)` is
/// emitted by the first `emit` at or after second 111. Until then, records of
/// second 100 still count in it. The agent stores its point at second 101,
/// the slot's end, however late it is sent.
///
/// # Records outside the open slots
///
/// Every record is counted exactly once:
/// - one older than the oldest open slot counts in that slot
///   ([`Stats::late_records`]);
/// - one from the future counts in the current slot
///   ([`Stats::future_records`]);
/// - after a stall (`emit` not called for a while, or the clock jumping
///   ahead), one `emit` emits at most 60 slots past the lateness. Slots older
///   than that are not lost: their counts are added into the first slot
///   emitted, which then shows them as one spike.
///
/// # Entities
///
/// - An entity starts with its first record. Its charts are defined, with
///   its labels, before their first update.
/// - After `expiry` without records its charts are marked obsolete and its
///   state is dropped; if it returns, its charts are defined again.
/// - Beyond the cap, records go to the overflow entity (see
///   [`Cap`](crate::Cap); [`Stats::overflowed_records`]).
pub struct Store {
    set: MetricSet,
    entities: HashMap<Labels, Entity>,
    overflow: Option<Entity>,
    /// Oldest slot not yet emitted; set on first use.
    next_emit: Option<u64>,
    stats: Stats,
}

impl Store {
    pub fn new(set: MetricSet) -> Self {
        Self {
            set,
            entities: HashMap::new(),
            overflow: None,
            next_emit: None,
            stats: Stats::default(),
        }
    }

    pub fn stats(&self) -> Stats {
        Stats {
            entities: self.entities.len() + usize::from(self.overflow.is_some()),
            ..self.stats
        }
    }

    /// The oldest slot not yet emitted, as of `now`: set on first use, and
    /// moved forward, folding the slots it passes, when it falls more than
    /// [`MAX_BACKLOG_SLOTS`] behind the lateness window.
    fn advance_window(&mut self, now: u64) -> u64 {
        let set = std::sync::Arc::clone(&self.set.0);
        let now_slot = now / set.interval_secs;
        let next_emit = *self
            .next_emit
            .get_or_insert(now_slot.saturating_sub(set.lateness_slots));
        let oldest_wanted = now_slot.saturating_sub(set.lateness_slots + MAX_BACKLOG_SLOTS);
        if next_emit >= oldest_wanted {
            return next_emit;
        }
        let skipped = oldest_wanted - next_emit;
        for entity in self.entities.values_mut().chain(self.overflow.as_mut()) {
            fold_front(&set, entity, skipped, oldest_wanted);
        }
        self.next_emit = Some(oldest_wanted);
        oldest_wanted
    }

    /// Merge a batch; `now`, the current Unix time, decides which slots are
    /// still open. Merge before an `emit` with the same `now`: a record whose
    /// slot that `emit` closes would otherwise count as late, in the next
    /// slot.
    ///
    /// # Panics
    ///
    /// If the batch is of another metric set.
    pub fn merge(&mut self, batch: Batch, now: u64) {
        assert!(
            std::sync::Arc::ptr_eq(&batch.set().0, &self.set.0),
            "batch of another metric set"
        );
        let next_emit = self.advance_window(now);
        let set = std::sync::Arc::clone(&self.set.0);
        let newest = (now / set.interval_secs).max(next_emit);
        for (labels, pending) in batch.entities {
            if pending.per_slot.is_empty() {
                continue;
            }
            let records: u64 = pending.per_slot.values().map(|s| s.records).sum();
            let entity = if let Some(entity) = self.entities.get_mut(&labels) {
                entity
            } else if self.entities.len() < set.max_entities {
                let entity = new_entity(&set, labels.clone(), false);
                self.entities.entry(labels).or_insert(entity)
            } else {
                self.stats.overflowed_records += records;
                self.overflow
                    .get_or_insert_with(|| new_entity(&set, set.overflow_labels.clone(), true))
            };
            for (slot, batch_cells) in pending.per_slot {
                let slot = if slot < next_emit {
                    self.stats.late_records += batch_cells.records;
                    next_emit
                } else if slot > newest {
                    self.stats.future_records += batch_cells.records;
                    newest
                } else {
                    slot
                };
                merge_slot(&set, entity, next_emit, slot, &batch_cells.cells);
            }
        }
    }

    /// Emit every slot whose lateness has passed by `now` (Unix seconds):
    /// the definitions of new entities' charts, one update per chart and
    /// slot, and the obsolete marks of expired entities.
    ///
    /// The text is complete chart-protocol text, to write to the agent as is
    /// (a plugin's stdout). It comes in chunks of at most `max_chunk` bytes,
    /// split only between whole charts, so each chunk can be written on its
    /// own, for example as one message over a size-limited channel; a single
    /// chart larger than `max_chunk` gets a chunk of its own. Pass that
    /// channel's limit, or `usize::MAX` when writing straight to stdout.
    pub fn emit(&mut self, now: u64, max_chunk: usize) -> Vec<Vec<u8>> {
        let mut out = Chunker::new(max_chunk);
        let mut next_emit = self.advance_window(now);
        let set = std::sync::Arc::clone(&self.set.0);
        let now_slot = now / set.interval_secs;
        while next_emit + set.lateness_slots < now_slot {
            let slot = next_emit;
            let mut expired = Vec::new();
            for (labels, entity) in &mut self.entities {
                if emit_slot(&set, entity, slot, &mut out) {
                    expired.push(labels.clone());
                }
            }
            for labels in expired {
                self.entities.remove(&labels);
                self.stats.expired_entities += 1;
            }
            if let Some(entity) = &mut self.overflow {
                if emit_slot(&set, entity, slot, &mut out) {
                    self.overflow = None;
                    self.stats.expired_entities += 1;
                }
            }
            next_emit += 1;
        }
        self.next_emit = Some(next_emit);
        out.finish()
    }
}

/// Merge the first `skipped` window slots of `entity` into the slot that
/// becomes the window's first, `new_first`.
fn fold_front(set: &MetricSetInner, entity: &mut Entity, skipped: u64, new_first: u64) {
    let take = (skipped as usize).min(entity.open_cells.len());
    if take == 0 {
        return;
    }
    let mut folded = set.initial_cells.clone().into_boxed_slice();
    for cells in entity.open_cells.drain(..take) {
        merge_cells(set, &mut folded, &cells);
    }
    match entity.open_cells.front_mut() {
        Some(front) => merge_cells(set, front, &folded),
        None => entity.open_cells.push_back(folded),
    }
    entity.first_slot = entity.first_slot.max(new_first);
}

fn merge_cells(set: &MetricSetInner, target: &mut [f64], cells: &[f64]) {
    for ((merge, current), &incoming) in set.merges.iter().zip(target.iter_mut()).zip(cells) {
        *current = merge.apply(*current, incoming);
    }
}

fn new_entity(set: &MetricSetInner, labels: Labels, overflow: bool) -> Entity {
    let suffix = if overflow {
        "_overflow".to_string()
    } else if labels.is_empty() {
        String::new()
    } else {
        format!("_{:016x}", label_hash(&labels))
    };
    Entity {
        chart_ids: set
            .charts
            .iter()
            .map(|chart| format!("{}{suffix}", chart.context))
            .collect(),
        labels,
        defined: false,
        first_slot: u64::MAX,
        last_slot: 0,
        open_cells: VecDeque::new(),
        previous_totals: vec![f64::NAN; set.cumulative_cells.len()].into_boxed_slice(),
    }
}

/// FNV-1a over the labels, which are sorted by key: stable across processes
/// and releases, so an entity keeps its chart ids over restarts.
fn label_hash(labels: &Labels) -> u64 {
    let mut hash: u64 = 0xcbf2_9ce4_8422_2325;
    for (key, value) in &labels.0 {
        for b in key.bytes().chain([0]).chain(value.bytes()).chain([0]) {
            hash ^= u64::from(b);
            hash = hash.wrapping_mul(0x0000_0100_0000_01b3);
        }
    }
    hash
}

fn merge_slot(set: &MetricSetInner, entity: &mut Entity, next_emit: u64, slot: u64, cells: &[f64]) {
    let index = (slot - next_emit) as usize;
    while entity.open_cells.len() <= index {
        entity
            .open_cells
            .push_back(set.initial_cells.clone().into_boxed_slice());
    }
    merge_cells(set, &mut entity.open_cells[index], cells);
    entity.first_slot = entity.first_slot.min(slot);
    entity.last_slot = entity.last_slot.max(slot);
}

/// Emit `slot` of one entity and drop it from the entity's window.
/// Returns true when the entity has expired; its charts are then already
/// marked obsolete in `out`.
fn emit_slot(set: &MetricSetInner, entity: &mut Entity, slot: u64, out: &mut Chunker) -> bool {
    let cells = entity.open_cells.pop_front();
    if slot >= entity.first_slot {
        if !entity.defined {
            for index in 0..set.charts.len() {
                define_chart(set, entity, index, false, out);
            }
            entity.defined = true;
        }
        let mut cells = cells.unwrap_or_else(|| set.initial_cells.clone().into_boxed_slice());
        totals_to_increases(set, entity, &mut cells);
        for (chart, id) in set.charts.iter().zip(&entity.chart_ids) {
            let block = out.block();
            write_begin(block, id, set.interval_secs * 1_000_000);
            for dim in &chart.dimensions {
                match dimension_value(set, &cells, dim.value) {
                    Some(value) => write_set_float(block, &dim.id, value),
                    None => write_set_empty(block, &dim.id),
                }
            }
            write_end_timed(block, (slot + 1) * set.interval_secs);
            out.commit();
        }
    }
    if entity.defined && slot >= entity.last_slot + set.expiry_slots {
        for index in 0..set.charts.len() {
            define_chart(set, entity, index, true, out);
        }
        return true;
    }
    false
}

/// Replace the totals in `cells` with their increase since the entity's
/// previous total, and remember the new totals.
fn totals_to_increases(set: &MetricSetInner, entity: &mut Entity, cells: &mut [f64]) {
    for (&cell, previous) in set
        .cumulative_cells
        .iter()
        .zip(entity.previous_totals.iter_mut())
    {
        let total = cells[cell];
        cells[cell] = if total.is_nan() {
            // No new total: no increase, once a baseline exists.
            if previous.is_nan() { f64::NAN } else { 0.0 }
        } else if previous.is_nan() {
            // The first total is the baseline.
            f64::NAN
        } else if total < *previous {
            // The source restarted from zero.
            total
        } else {
            total - *previous
        };
        if !total.is_nan() {
            *previous = total;
        }
    }
}

fn dimension_value(set: &MetricSetInner, cells: &[f64], value: Value) -> Option<f64> {
    let value = match value {
        Value::Rate(cell) => cells[cell] / set.interval_secs as f64,
        Value::Ratio {
            numerator,
            denominator,
        } if cells[denominator] > 0.0 => cells[numerator] / cells[denominator],
        Value::Ratio { .. } => return None,
        Value::Cell(cell) => cells[cell],
    };
    value.is_finite().then_some(value)
}

/// One chart block: the CHART line and, unless the chart is being marked
/// obsolete, its labels and dimensions. `store_first` aligns the agent's
/// clock for the chart to whole seconds and keeps its first point, so every
/// timed END lands exactly on its slot.
fn define_chart(
    set: &MetricSetInner,
    entity: &Entity,
    index: usize,
    obsolete: bool,
    out: &mut Chunker,
) {
    let chart = &set.charts[index];
    let block = out.block();
    write_chart(
        block,
        &ChartLine {
            id: &entity.chart_ids[index],
            name: "",
            title: &chart.title,
            units: chart.units,
            family: chart.family,
            context: &chart.context,
            chart_type: chart.chart_type,
            priority: chart.priority,
            update_every: set.interval_secs,
            options: ChartOptions {
                obsolete,
                store_first: true,
                hidden: false,
            },
        },
    );
    if !obsolete {
        if !entity.labels.is_empty() {
            for (key, value) in &entity.labels.0 {
                write_clabel(block, key, value, LabelSource::Auto);
            }
            write_clabel_commit(block);
        }
        for dim in &chart.dimensions {
            write_dimension(
                block,
                &DimensionLine {
                    id: &dim.id,
                    name: &dim.id,
                    algorithm: DimensionAlgorithm::Absolute,
                    multiplier: 1,
                    divisor: 1,
                    options: DimensionOptions {
                        float: true,
                        ..DimensionOptions::default()
                    },
                },
            );
        }
    }
    out.commit();
}

/// Packs chart blocks into chunks under a byte budget.
struct Chunker {
    max_chunk: usize,
    chunks: Vec<Vec<u8>>,
    current: Vec<u8>,
    block: Vec<u8>,
}

impl Chunker {
    fn new(max_chunk: usize) -> Self {
        Self {
            max_chunk,
            chunks: Vec::new(),
            current: Vec::new(),
            block: Vec::new(),
        }
    }

    /// The buffer for the next block; [`Chunker::commit`] files it.
    fn block(&mut self) -> &mut Vec<u8> {
        self.block.clear();
        &mut self.block
    }

    fn commit(&mut self) {
        if !self.current.is_empty() && self.current.len() + self.block.len() > self.max_chunk {
            self.chunks.push(std::mem::take(&mut self.current));
        }
        self.current.extend_from_slice(&self.block);
    }

    fn finish(mut self) -> Vec<Vec<u8>> {
        if !self.current.is_empty() {
            self.chunks.push(self.current);
        }
        self.chunks
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{Cap, Chart, ChartType, Counter, Histogram, MetricSetBuilder, Settings};
    use netdata_plugin_protocol::validate_chart_stream;
    use std::time::Duration;

    const BOUNDS: &[f64] = &[0.1, 1.0];

    crate::dimensions! {
        enum Outcome {
            Ok = "ok",
            Error = "error",
        }
    }

    struct Fixture {
        set: MetricSet,
        requests: Counter<Outcome>,
        duration: Histogram,
    }

    fn fixture(lateness: u64, expiry: u64, max: Option<usize>) -> Fixture {
        let cap = max.map(|max| {
            let mut overflow_labels = Labels::new();
            overflow_labels.push("overflow", "true");
            Cap {
                max,
                overflow_labels,
            }
        });
        let mut b = MetricSetBuilder::new(Settings {
            lateness: Duration::from_secs(lateness),
            expiry: Duration::from_secs(expiry),
            cap,
            ..Settings::default()
        });
        let requests = b.counter::<Outcome>(Chart {
            title: "Requests",
            units: "requests/s",
            family: "requests",
            context: "svc.requests",
            chart_type: ChartType::Stacked,
            priority: 1,
        });
        let duration = b.histogram(
            BOUNDS,
            Chart {
                title: "Duration",
                units: "seconds",
                family: "requests",
                context: "svc.duration",
                chart_type: ChartType::Line,
                priority: 2,
            },
            "requests/s",
        );
        Fixture {
            set: b.build(),
            requests,
            duration,
        }
    }

    fn labels(name: &str) -> Labels {
        let mut labels = Labels::new();
        labels.push("service.name", name);
        labels
    }

    fn text(chunks: &[Vec<u8>]) -> String {
        String::from_utf8(chunks.concat()).unwrap()
    }

    /// Records `count` requests of `name` at `time`, the first `errors` of them
    /// failed, each taking half a second.
    fn record(
        f: &Fixture,
        store: &mut Store,
        name: &str,
        time: u64,
        count: u64,
        errors: u64,
        now: u64,
    ) {
        let mut batch = Batch::new(&f.set);
        let mut entity = batch.entity(labels(name));
        for i in 0..count {
            entity
                .record(time)
                .add(
                    f.requests,
                    if i < errors {
                        Outcome::Error
                    } else {
                        Outcome::Ok
                    },
                    1.0,
                )
                .observe(f.duration, 0.5, 1.0);
        }
        store.merge(batch, now);
    }

    #[test]
    fn defines_once_then_updates_each_slot() {
        let f = fixture(0, 900, None);
        let mut store = Store::new(f.set.clone());
        record(&f, &mut store, "checkout", 100, 3, 1, 100);
        let first = text(&store.emit(101, usize::MAX));
        let h = format!("{:016x}", label_hash(&labels("checkout")));
        assert_eq!(
            first,
            format!(
                "CHART svc.requests_{h} '' 'Requests' 'requests/s' 'requests' 'svc.requests' stacked 1 1 'store_first'\n\
                 CLABEL 'service.name' 'checkout' 1\nCLABEL_COMMIT\n\
                 DIMENSION ok 'ok' absolute 1 1 'type=float'\n\
                 DIMENSION error 'error' absolute 1 1 'type=float'\n\
                 CHART svc.duration_{h} '' 'Duration' 'seconds' 'requests' 'svc.duration' line 2 1 'store_first'\n\
                 CLABEL 'service.name' 'checkout' 1\nCLABEL_COMMIT\n\
                 DIMENSION avg 'avg' absolute 1 1 'type=float'\n\
                 DIMENSION max 'max' absolute 1 1 'type=float'\n\
                 CHART svc.duration_histogram_{h} '' 'Duration histogram' 'requests/s' 'requests' \
                 'svc.duration_histogram' heatmap 3 1 'store_first'\n\
                 CLABEL 'service.name' 'checkout' 1\nCLABEL_COMMIT\n\
                 DIMENSION 0.1 '0.1' absolute 1 1 'type=float'\n\
                 DIMENSION 1 '1' absolute 1 1 'type=float'\n\
                 DIMENSION +Inf '+Inf' absolute 1 1 'type=float'\n\
                 BEGIN svc.requests_{h} 1000000\nSET ok = 2\nSET error = 1\nEND 101 0 1\n\
                 BEGIN svc.duration_{h} 1000000\nSET avg = 0.5\nSET max = 0.5\nEND 101 0 1\n\
                 BEGIN svc.duration_histogram_{h} 1000000\nSET 0.1 = 0\nSET 1 = 3\nSET +Inf = 0\nEND 101 0 1\n"
            )
        );
        // The next slot: zero rates, no average or maximum, no new definition.
        let second = text(&store.emit(102, usize::MAX));
        assert_eq!(
            second,
            format!(
                "BEGIN svc.requests_{h} 1000000\nSET ok = 0\nSET error = 0\nEND 102 0 1\n\
                 BEGIN svc.duration_{h} 1000000\nSET avg =\nSET max =\nEND 102 0 1\n\
                 BEGIN svc.duration_histogram_{h} 1000000\nSET 0.1 = 0\nSET 1 = 0\nSET +Inf = 0\nEND 102 0 1\n"
            )
        );
        validate_chart_stream(format!("{first}{second}").as_bytes()).unwrap();
        // Nothing is emitted twice.
        assert!(store.emit(102, usize::MAX).is_empty());
    }

    #[test]
    fn lateness_keeps_slots_open_and_clamps_exactly_once() {
        let f = fixture(10, 900, None);
        let mut store = Store::new(f.set.clone());
        // Window starts at slot 990 (now 1000, lateness 10).
        record(&f, &mut store, "a", 995, 1, 0, 1000);
        // Too late: counted in the oldest open slot.
        record(&f, &mut store, "a", 100, 2, 0, 1000);
        // From the future: counted in the current slot.
        record(&f, &mut store, "a", 5000, 4, 0, 1000);
        assert_eq!(store.stats().late_records, 2);
        assert_eq!(store.stats().future_records, 4);
        // Slot 999 is not finished plus lateness until 1010.
        let out = text(&store.emit(1009, usize::MAX));
        validate_chart_stream(out.as_bytes()).unwrap();
        let ok_values = |out: &str| -> Vec<f64> {
            out.lines()
                .filter_map(|l| l.strip_prefix("SET ok = "))
                .map(|v| v.parse().unwrap())
                .collect()
        };
        // Slots 990..=998: the clamped 2 land in 990, the record at 995.
        let emitted = ok_values(&out);
        assert_eq!(emitted.len(), 9);
        assert_eq!(emitted[0], 2.0);
        assert_eq!(emitted[5], 1.0);
        let rest = text(&store.emit(1011, usize::MAX));
        // Slots 999 and 1000: the 4 future records land in slot 1000.
        assert_eq!(ok_values(&rest), vec![0.0, 4.0]);
        validate_chart_stream(format!("{out}{rest}").as_bytes()).unwrap();
    }

    #[test]
    fn expired_entities_are_marked_obsolete_and_revived() {
        let f = fixture(0, 3, None);
        let mut store = Store::new(f.set.clone());
        record(&f, &mut store, "a", 10, 1, 0, 10);
        let mut out = text(&store.emit(13, usize::MAX));
        assert!(!out.contains("obsolete"));
        out += &text(&store.emit(14, usize::MAX));
        assert_eq!(out.matches("'obsolete store_first'").count(), 3);
        assert_eq!(store.stats().entities, 0);
        assert_eq!(store.stats().expired_entities, 1);
        record(&f, &mut store, "a", 20, 1, 0, 20);
        let again = text(&store.emit(21, usize::MAX));
        assert_eq!(again.matches("CHART ").count(), 3);
        assert!(!again.contains("obsolete"));
        assert_eq!(again.matches("CLABEL_COMMIT").count(), 3);
        validate_chart_stream(format!("{out}{again}").as_bytes()).unwrap();
    }

    #[test]
    fn entities_over_the_cap_share_the_overflow_entity() {
        let f = fixture(0, 3, Some(1));
        let mut store = Store::new(f.set.clone());
        record(&f, &mut store, "a", 10, 1, 0, 10);
        record(&f, &mut store, "b", 10, 2, 0, 10);
        record(&f, &mut store, "c", 10, 3, 0, 10);
        let stats = store.stats();
        assert_eq!(stats.entities, 2);
        assert_eq!(stats.overflowed_records, 5);
        let out = text(&store.emit(11, usize::MAX));
        assert!(out.contains("BEGIN svc.requests_overflow 1000000\nSET ok = 5\n"));
        assert!(out.contains("CLABEL 'overflow' 'true' 1\n"));
        // Once "a" and the overflow entity expire, "b" gets its own charts.
        let expired = text(&store.emit(14, usize::MAX));
        assert_eq!(store.stats().entities, 0);
        record(&f, &mut store, "b", 20, 1, 0, 20);
        let b = text(&store.emit(21, usize::MAX));
        assert!(b.contains("CLABEL 'service.name' 'b' 1"));
        validate_chart_stream(format!("{out}{expired}{b}").as_bytes()).unwrap();
    }

    #[test]
    fn entities_without_records_are_not_created() {
        let f = fixture(0, 3, Some(1));
        let mut store = Store::new(f.set.clone());
        let mut batch = Batch::new(&f.set);
        batch.entity(labels("empty"));
        store.merge(batch, 10);
        assert_eq!(store.stats().entities, 0);
        // The cap is still free for an entity with records.
        record(&f, &mut store, "a", 10, 1, 0, 10);
        assert_eq!(store.stats().overflowed_records, 0);
        let out = text(&store.emit(11, usize::MAX));
        assert!(out.contains("CLABEL 'service.name' 'a' 1"));
        assert!(!out.contains("'empty'"));
    }

    #[test]
    fn a_clock_jump_renders_a_bounded_backlog_and_keeps_every_record() {
        let f = fixture(10, 1_000_000, None);
        let mut store = Store::new(f.set.clone());
        record(&f, &mut store, "a", 10, 1, 0, 10);
        record(&f, &mut store, "a", 100_000, 2, 0, 100_000);
        let window = store.entities.values().next().unwrap().open_cells.len() as u64;
        assert!(window <= 10 + MAX_BACKLOG_SLOTS + 1, "{window}");
        let out = text(&store.emit(100_011, usize::MAX));
        validate_chart_stream(out.as_bytes()).unwrap();
        let updates = out.matches("BEGIN ").count() as u64 / 3;
        assert!(updates <= MAX_BACKLOG_SLOTS + 11, "{updates}");
        let ok: f64 = out
            .lines()
            .filter_map(|l| l.strip_prefix("SET ok = "))
            .map(|v| v.parse::<f64>().unwrap())
            .sum();
        assert_eq!(ok, 3.0);
    }

    #[test]
    fn records_spread_over_many_seconds_land_in_their_slots() {
        let f = fixture(10, 900, None);
        let mut store = Store::new(f.set.clone());
        let mut batch = Batch::new(&f.set);
        let mut entity = batch.entity(labels("a"));
        for time in [995, 996, 995, 997, 996, 999] {
            entity.record(time).add(f.requests, Outcome::Ok, 1.0);
        }
        store.merge(batch, 1000);
        let out = text(&store.emit(1011, usize::MAX));
        let ok: Vec<&str> = out
            .lines()
            .filter_map(|l| l.strip_prefix("SET ok = "))
            .collect();
        assert_eq!(ok, ["2", "2", "1", "0", "1", "0"]);
    }

    #[test]
    fn chunks_hold_whole_blocks_under_the_budget() {
        let f = fixture(0, 900, None);
        let mut store = Store::new(f.set.clone());
        for name in ["a", "b", "c", "d"] {
            record(&f, &mut store, name, 10, 1, 0, 10);
        }
        let chunks = store.emit(12, 400);
        assert!(chunks.len() > 1);
        for chunk in &chunks {
            let text = std::str::from_utf8(chunk).unwrap();
            assert!(
                text.starts_with("CHART ") || text.starts_with("BEGIN "),
                "{text}"
            );
            assert!(chunk.len() <= 400 || !text[..text.len() - 1].contains("\nCHART "));
        }
        validate_chart_stream(&chunks.concat()).unwrap();
    }

    #[test]
    fn labels_drop_empty_values_and_cut_long_ones_on_a_character() {
        let mut labels = Labels::new();
        labels.push("empty", "");
        labels.push("blank", " \t\n");
        labels.push("long", &"é".repeat(500));
        assert_eq!(labels.0.len(), 1);
        assert_eq!(labels.0[0].1.len(), 800);
        let f = fixture(0, 900, None);
        let entity = new_entity(&f.set.0, labels, false);
        assert!(entity.chart_ids[0].starts_with("svc.requests_"));
        assert!(netdata_plugin_protocol::is_valid_chart_id(
            &entity.chart_ids[0]
        ));
    }

    #[test]
    fn push_order_does_not_change_the_entity_or_its_chart_ids() {
        let mut ab = Labels::new();
        ab.push("a", "1");
        ab.push("b", "2");
        let mut ba = Labels::new();
        ba.push("b", "2");
        ba.push("a", "1");
        assert_eq!(ab, ba);
        let f = fixture(0, 900, None);
        let ids = |labels: Labels| new_entity(&f.set.0, labels, false).chart_ids;
        assert_eq!(ids(ab), ids(ba));
        assert_eq!(ids(Labels::new())[0], "svc.requests");
    }

    #[test]
    fn cumulative_totals_render_as_increases() {
        crate::dimensions! {
            enum Events {
                Events = "events",
            }
        }
        let mut b = MetricSetBuilder::new(Settings {
            expiry: Duration::from_secs(3),
            ..Settings::default()
        });
        let totals = b.cumulative_counter::<Events>(Chart {
            title: "Events",
            units: "events/s",
            family: "events",
            context: "src.events",
            chart_type: ChartType::Line,
            priority: 1,
        });
        let set = b.build();
        let mut store = Store::new(set.clone());
        let set = |store: &mut Store, time: u64, total: Option<f64>| {
            let mut batch = Batch::new(&set);
            let mut entity = batch.entity(Labels::new());
            let mut record = entity.record(time);
            if let Some(total) = total {
                record.set_total(totals, Events::Events, total);
            }
            store.merge(batch, time);
        };
        // Baseline, two totals in one slot (the last wins), no new total, a
        // reset, then a return after expiry with a new baseline.
        set(&mut store, 10, Some(100.0));
        set(&mut store, 11, Some(103.0));
        set(&mut store, 11, Some(107.0));
        set(&mut store, 12, None);
        set(&mut store, 13, Some(5.0));
        let mut out = text(&store.emit(14, usize::MAX));
        out += &text(&store.emit(18, usize::MAX));
        set(&mut store, 20, Some(50.0));
        set(&mut store, 21, Some(52.0));
        out += &text(&store.emit(22, usize::MAX));
        validate_chart_stream(out.as_bytes()).unwrap();
        let values: Vec<&str> = out
            .lines()
            .filter_map(|l| l.strip_prefix("SET events ="))
            .collect();
        assert_eq!(values, ["", " 7", " 0", " 5", " 0", " 0", " 0", "", " 2"]);
        assert_eq!(out.matches("'obsolete store_first'").count(), 1);
    }

    #[test]
    #[should_panic(expected = "instrument of another metric set")]
    fn an_instrument_of_another_metric_set_is_refused() {
        let f = fixture(0, 900, None);
        let other = fixture(0, 900, None);
        let mut batch = Batch::new(&f.set);
        let mut entity = batch.entity(labels("a"));
        entity.record(10).add(other.requests, Outcome::Ok, 1.0);
    }

    #[test]
    #[should_panic(expected = "dimension index 2 out of 2")]
    fn a_dimension_index_out_of_range_is_refused() {
        #[derive(Clone, Copy)]
        struct Broken;
        impl crate::Dimensions for Broken {
            const IDS: &'static [&'static str] = &["a", "b"];
            fn index(self) -> usize {
                2
            }
        }
        let mut b = MetricSetBuilder::new(Settings::default());
        let counter = b.counter::<Broken>(Chart {
            title: "C",
            units: "events/s",
            family: "c",
            context: "x.c",
            chart_type: ChartType::Line,
            priority: 1,
        });
        let set = b.build();
        let mut batch = Batch::new(&set);
        let mut entity = batch.entity(Labels::new());
        entity.record(10).add(counter, Broken, 1.0);
    }

    #[test]
    #[should_panic(expected = "the collection interval must be whole seconds")]
    fn a_fractional_collection_interval_is_refused() {
        MetricSetBuilder::new(Settings {
            collection_interval: Duration::from_millis(1500),
            ..Settings::default()
        });
    }

    #[test]
    fn a_batch_without_records_is_empty() {
        let f = fixture(0, 900, None);
        let mut batch = Batch::new(&f.set);
        batch.entity(labels("a"));
        assert!(batch.is_empty());
        batch.entity(labels("a")).record(10);
        assert!(!batch.is_empty());
    }

    #[test]
    fn weights_scale_counts_and_histograms() {
        let f = fixture(0, 900, None);
        let mut store = Store::new(f.set.clone());
        let mut batch = Batch::new(&f.set);
        let mut entity = batch.entity(labels("a"));
        entity
            .record(10)
            .add(f.requests, Outcome::Ok, 2.0)
            .observe(f.duration, 0.25, 2.0);
        entity
            .record(10)
            .add(f.requests, Outcome::Error, 1.0)
            .observe(f.duration, 2.5, 1.0);
        store.merge(batch, 10);
        let out = text(&store.emit(11, usize::MAX));
        assert!(out.contains("SET ok = 2\nSET error = 1\n"));
        assert!(out.contains("SET avg = 1\nSET max = 2.5\n"));
        assert!(out.contains("SET 0.1 = 0\nSET 1 = 2\nSET +Inf = 1\n"));
    }
}
