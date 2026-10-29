//! The store: instances, the open-slot window, the instance cap, expiry and
//! rendering.
//!
//! Slots are numbered `unix_seconds / update_every`; slot `s` covers
//! `[s * update_every, (s + 1) * update_every)` and its point is stored at
//! its end. The store keeps every slot from `next_emit` (the oldest slot not
//! yet rendered) to the current one open. A slot is rendered once the
//! family's lateness has passed after its end, and never again; a record
//! older than the window is counted in the oldest open slot and one from the
//! future in the current slot, so every record is counted exactly once.
//!
//! Charts are defined with `store_first` and every update ends with
//! `END <slot end> 0 1`, so the agent stores each slot's point at the slot's
//! end, not at the time the store renders it (see
//! `netdata_plugin_protocol::write_end_timed`).
//!
//! After a stall or a forward step of the clock, at most
//! [`MAX_BACKLOG_SLOTS`] slots beyond the lateness are rendered at once:
//! pending slots older than that are folded into the oldest one rendered, so
//! no record is lost and memory stays bounded.

use crate::batch::Batch;
use crate::family::{Family, FamilyInner, Labels, Value, is_id_byte};
use netdata_plugin_protocol::{
    ChartLine, ChartOptions, DimensionAlgorithm, DimensionLine, DimensionOptions, LabelSource,
    write_begin, write_chart, write_clabel, write_clabel_commit, write_dimension, write_end_timed,
    write_set_empty, write_set_float,
};
use std::collections::{HashMap, VecDeque};

/// Readable part of an instance's chart type, before the hash.
const MAX_NAME_BYTES: usize = 48;

/// Slots beyond the lateness that one catch-up renders at most.
pub const MAX_BACKLOG_SLOTS: u64 = 60;

/// Counts since the store was created; `instances` is the current number.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct Stats {
    /// Instances currently tracked, the overflow instance included.
    pub instances: usize,
    /// Records counted in the overflow instance because of the cap.
    pub overflowed_records: u64,
    /// Records older than the open window, counted in its oldest slot.
    pub late_records: u64,
    /// Records newer than the current slot, counted in the current slot.
    pub future_records: u64,
    /// Instances whose charts were marked obsolete after the expiry time.
    pub expired_instances: u64,
}

struct Instance {
    labels: Labels,
    /// The full id of each of the family's charts, in family order.
    chart_ids: Vec<String>,
    defined: bool,
    /// Earliest slot with records; nothing is rendered before it.
    first_slot: u64,
    /// Latest slot with records; drives expiry.
    last_slot: u64,
    /// Open slots from `next_emit` onwards; missing ones are empty.
    slots: VecDeque<Box<[f64]>>,
}

/// Accumulates batches of one family and renders them as chart-protocol text.
pub struct Store {
    family: Family,
    instances: HashMap<Labels, Instance>,
    overflow: Option<Instance>,
    /// Oldest slot not yet rendered; set on first use.
    next_emit: Option<u64>,
    stats: Stats,
}

impl Store {
    pub fn new(family: Family) -> Self {
        Self {
            family,
            instances: HashMap::new(),
            overflow: None,
            next_emit: None,
            stats: Stats::default(),
        }
    }

    pub fn stats(&self) -> Stats {
        Stats {
            instances: self.instances.len() + usize::from(self.overflow.is_some()),
            ..self.stats
        }
    }

    /// The oldest slot not yet rendered, as of `now`: set on first use, and
    /// moved forward when it falls more than [`MAX_BACKLOG_SLOTS`] behind the
    /// lateness window.
    fn start(&mut self, now: u64) -> u64 {
        let family = std::sync::Arc::clone(&self.family.0);
        let now_slot = now / family.update_every;
        let next_emit = *self
            .next_emit
            .get_or_insert(now_slot.saturating_sub(family.lateness_slots));
        let oldest_wanted = now_slot.saturating_sub(family.lateness_slots + MAX_BACKLOG_SLOTS);
        if next_emit >= oldest_wanted {
            return next_emit;
        }
        let skipped = oldest_wanted - next_emit;
        for instance in self.instances.values_mut().chain(self.overflow.as_mut()) {
            fold_front(&family, instance, skipped, oldest_wanted);
        }
        self.next_emit = Some(oldest_wanted);
        oldest_wanted
    }

    /// Merge a batch recorded at or before `now` (Unix seconds).
    pub fn merge(&mut self, batch: Batch, now: u64) {
        assert!(
            std::sync::Arc::ptr_eq(&batch.family().0, &self.family.0),
            "batch of another family"
        );
        let next_emit = self.start(now);
        let family = std::sync::Arc::clone(&self.family.0);
        let newest = (now / family.update_every).max(next_emit);
        for batch_instance in batch.instances {
            if batch_instance.slots.is_empty() {
                continue;
            }
            let records: u64 = batch_instance.slots.iter().map(|s| s.records).sum();
            let instance = if let Some(instance) = self.instances.get_mut(&batch_instance.labels) {
                instance
            } else if self.instances.len() < family.max_instances {
                let instance = new_instance(&family, batch_instance.labels.clone(), false);
                self.instances
                    .entry(batch_instance.labels)
                    .or_insert(instance)
            } else {
                self.stats.overflowed_records += records;
                self.overflow.get_or_insert_with(|| {
                    new_instance(&family, family.overflow_labels.clone(), true)
                })
            };
            for partial in batch_instance.slots {
                let slot = if partial.slot < next_emit {
                    self.stats.late_records += partial.records;
                    next_emit
                } else if partial.slot > newest {
                    self.stats.future_records += partial.records;
                    newest
                } else {
                    partial.slot
                };
                merge_slot(&family, instance, next_emit, slot, &partial.cells);
            }
        }
    }

    /// Render every slot whose lateness has passed by `now` (Unix seconds):
    /// definitions of new instances, one update per chart and slot, and
    /// obsolete charts of expired instances. Returns chunks of at most
    /// `max_chunk` bytes, each a run of whole chart blocks (a single block
    /// larger than `max_chunk` gets a chunk of its own).
    pub fn emit(&mut self, now: u64, max_chunk: usize) -> Vec<Vec<u8>> {
        let mut out = Chunker::new(max_chunk);
        let mut next_emit = self.start(now);
        let family = std::sync::Arc::clone(&self.family.0);
        let now_slot = now / family.update_every;
        while next_emit + family.lateness_slots < now_slot {
            let slot = next_emit;
            let mut expired = Vec::new();
            for (labels, instance) in &mut self.instances {
                if emit_slot(&family, instance, slot, &mut out) {
                    expired.push(labels.clone());
                }
            }
            for labels in expired {
                self.instances.remove(&labels);
                self.stats.expired_instances += 1;
            }
            if let Some(instance) = &mut self.overflow {
                if emit_slot(&family, instance, slot, &mut out) {
                    self.overflow = None;
                    self.stats.expired_instances += 1;
                }
            }
            next_emit += 1;
        }
        self.next_emit = Some(next_emit);
        out.finish()
    }
}

/// Merge the first `skipped` window slots of `instance` into the slot that
/// becomes the window's first, `new_first`.
fn fold_front(family: &FamilyInner, instance: &mut Instance, skipped: u64, new_first: u64) {
    let take = (skipped as usize).min(instance.slots.len());
    if take == 0 {
        return;
    }
    let mut folded = family.initial.clone().into_boxed_slice();
    for cells in instance.slots.drain(..take) {
        merge_cells(family, &mut folded, &cells);
    }
    match instance.slots.front_mut() {
        Some(front) => merge_cells(family, front, &folded),
        None => instance.slots.push_back(folded),
    }
    instance.first_slot = instance.first_slot.max(new_first);
}

fn merge_cells(family: &FamilyInner, target: &mut [f64], cells: &[f64]) {
    for ((merge, current), &incoming) in family.merges.iter().zip(target.iter_mut()).zip(cells) {
        *current = merge.apply(*current, incoming);
    }
}

fn new_instance(family: &FamilyInner, labels: Labels, overflow: bool) -> Instance {
    let chart_type_id = if labels.is_empty() {
        family.type_prefix.clone()
    } else if overflow {
        format!("{}_overflow", family.type_prefix)
    } else {
        let mut name = String::with_capacity(MAX_NAME_BYTES);
        for b in labels.0[0].1.bytes().take(MAX_NAME_BYTES) {
            name.push(if is_id_byte(b) { b as char } else { '_' });
        }
        format!(
            "{}_{}_{:016x}",
            family.type_prefix,
            name,
            label_hash(&labels)
        )
    };
    Instance {
        chart_ids: family
            .charts
            .iter()
            .map(|chart| format!("{chart_type_id}.{}", chart.id))
            .collect(),
        labels,
        defined: false,
        first_slot: u64::MAX,
        last_slot: 0,
        slots: VecDeque::new(),
    }
}

/// FNV-1a over the labels: stable across processes and releases, so an
/// instance keeps its chart ids over restarts.
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

fn merge_slot(
    family: &FamilyInner,
    instance: &mut Instance,
    next_emit: u64,
    slot: u64,
    cells: &[f64],
) {
    let index = (slot - next_emit) as usize;
    while instance.slots.len() <= index {
        instance
            .slots
            .push_back(family.initial.clone().into_boxed_slice());
    }
    merge_cells(family, &mut instance.slots[index], cells);
    instance.first_slot = instance.first_slot.min(slot);
    instance.last_slot = instance.last_slot.max(slot);
}

/// Render `slot` of one instance and drop it from the instance's window.
/// Returns true when the instance has expired; its charts are then already
/// marked obsolete in `out`.
fn emit_slot(family: &FamilyInner, instance: &mut Instance, slot: u64, out: &mut Chunker) -> bool {
    let cells = instance.slots.pop_front();
    if slot >= instance.first_slot {
        if !instance.defined {
            for index in 0..family.charts.len() {
                define_chart(family, instance, index, false, out);
            }
            instance.defined = true;
        }
        let cells = cells.unwrap_or_else(|| family.initial.clone().into_boxed_slice());
        for (chart, id) in family.charts.iter().zip(&instance.chart_ids) {
            let block = out.block();
            write_begin(block, id, family.update_every * 1_000_000);
            for dim in &chart.dimensions {
                match render(family, &cells, dim.value) {
                    Some(value) => write_set_float(block, &dim.id, value),
                    None => write_set_empty(block, &dim.id),
                }
            }
            write_end_timed(block, (slot + 1) * family.update_every);
            out.commit();
        }
    }
    if instance.defined && slot >= instance.last_slot + family.expiry_slots {
        for index in 0..family.charts.len() {
            define_chart(family, instance, index, true, out);
        }
        return true;
    }
    false
}

fn render(family: &FamilyInner, cells: &[f64], value: Value) -> Option<f64> {
    let value = match value {
        Value::Rate(cell) => cells[cell] / family.update_every as f64,
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
    family: &FamilyInner,
    instance: &Instance,
    index: usize,
    obsolete: bool,
    out: &mut Chunker,
) {
    let chart = &family.charts[index];
    let block = out.block();
    write_chart(
        block,
        &ChartLine {
            id: &instance.chart_ids[index],
            name: "",
            title: chart.title,
            units: chart.units,
            family: chart.family,
            context: chart.context,
            chart_type: chart.chart_type,
            priority: chart.priority,
            update_every: family.update_every,
            options: ChartOptions {
                obsolete,
                store_first: true,
                hidden: false,
            },
        },
    );
    if !obsolete {
        if !instance.labels.is_empty() {
            for (key, value) in &instance.labels.0 {
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
    use crate::{ChartSpec, ChartType, Counter, FamilyBuilder, Histogram};
    use netdata_plugin_protocol::validate_chart_stream;

    const BOUNDS: &[f64] = &[0.1, 1.0];

    struct Fixture {
        family: Family,
        requests: Counter,
        duration: Histogram,
    }

    fn fixture(lateness: u64, expiry: u64, max: Option<usize>) -> Fixture {
        let mut b = FamilyBuilder::new("svc").lateness(lateness).expiry(expiry);
        if let Some(max) = max {
            let mut overflow = Labels::new();
            overflow.push("overflow", "true");
            b = b.max_instances(max, overflow);
        }
        let requests = b.counter(2);
        let duration = b.histogram(BOUNDS);
        b.chart(ChartSpec {
            id: "requests",
            title: "Requests",
            units: "requests/s",
            family: "requests",
            context: "svc.requests",
            chart_type: ChartType::Stacked,
            priority: 1,
            dimensions: vec![requests.rate("ok", 0), requests.rate("error", 1)],
        });
        b.chart(ChartSpec {
            id: "duration",
            title: "Duration",
            units: "seconds",
            family: "requests",
            context: "svc.duration",
            chart_type: ChartType::Line,
            priority: 2,
            dimensions: vec![duration.average("avg"), duration.maximum("max")],
        });
        b.chart(ChartSpec {
            id: "duration_histogram",
            title: "Duration histogram",
            units: "requests/s",
            family: "requests",
            context: "svc.duration_histogram",
            chart_type: ChartType::Heatmap,
            priority: 3,
            dimensions: duration.bucket_rates(),
        });
        Fixture {
            family: b.build(),
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
        let mut batch = Batch::new(&f.family);
        let inst = batch.instance(labels(name));
        for i in 0..count {
            batch
                .record(inst, time)
                .add(f.requests, usize::from(i < errors), 1.0)
                .observe(f.duration, 0.5, 1.0);
        }
        store.merge(batch, now);
    }

    #[test]
    fn defines_once_then_updates_each_slot() {
        let f = fixture(0, 900, None);
        let mut store = Store::new(f.family.clone());
        record(&f, &mut store, "checkout", 100, 3, 1, 100);
        let first = text(&store.emit(101, usize::MAX));
        let id = format!("svc_checkout_{:08x}", label_hash(&labels("checkout")));
        assert_eq!(
            first,
            format!(
                "CHART {id}.requests '' 'Requests' 'requests/s' 'requests' 'svc.requests' stacked 1 1 'store_first'\n\
                 CLABEL 'service.name' 'checkout' 1\nCLABEL_COMMIT\n\
                 DIMENSION ok 'ok' absolute 1 1 'type=float'\n\
                 DIMENSION error 'error' absolute 1 1 'type=float'\n\
                 CHART {id}.duration '' 'Duration' 'seconds' 'requests' 'svc.duration' line 2 1 'store_first'\n\
                 CLABEL 'service.name' 'checkout' 1\nCLABEL_COMMIT\n\
                 DIMENSION avg 'avg' absolute 1 1 'type=float'\n\
                 DIMENSION max 'max' absolute 1 1 'type=float'\n\
                 CHART {id}.duration_histogram '' 'Duration histogram' 'requests/s' 'requests' \
                 'svc.duration_histogram' heatmap 3 1 'store_first'\n\
                 CLABEL 'service.name' 'checkout' 1\nCLABEL_COMMIT\n\
                 DIMENSION 0.1 '0.1' absolute 1 1 'type=float'\n\
                 DIMENSION 1 '1' absolute 1 1 'type=float'\n\
                 DIMENSION +Inf '+Inf' absolute 1 1 'type=float'\n\
                 BEGIN {id}.requests 1000000\nSET ok = 2\nSET error = 1\nEND 101 0 1\n\
                 BEGIN {id}.duration 1000000\nSET avg = 0.5\nSET max = 0.5\nEND 101 0 1\n\
                 BEGIN {id}.duration_histogram 1000000\nSET 0.1 = 0\nSET 1 = 3\nSET +Inf = 0\nEND 101 0 1\n"
            )
        );
        // The next slot: zero rates, no average or maximum, no new definition.
        let second = text(&store.emit(102, usize::MAX));
        assert_eq!(
            second,
            format!(
                "BEGIN {id}.requests 1000000\nSET ok = 0\nSET error = 0\nEND 102 0 1\n\
                 BEGIN {id}.duration 1000000\nSET avg =\nSET max =\nEND 102 0 1\n\
                 BEGIN {id}.duration_histogram 1000000\nSET 0.1 = 0\nSET 1 = 0\nSET +Inf = 0\nEND 102 0 1\n"
            )
        );
        validate_chart_stream(format!("{first}{second}").as_bytes()).unwrap();
        // Nothing is rendered twice.
        assert!(store.emit(102, usize::MAX).is_empty());
    }

    #[test]
    fn lateness_keeps_slots_open_and_clamps_exactly_once() {
        let f = fixture(10, 900, None);
        let mut store = Store::new(f.family.clone());
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
    fn expired_instances_are_marked_obsolete_and_revived() {
        let f = fixture(0, 3, None);
        let mut store = Store::new(f.family.clone());
        record(&f, &mut store, "a", 10, 1, 0, 10);
        let mut out = text(&store.emit(13, usize::MAX));
        assert!(!out.contains("obsolete"));
        out += &text(&store.emit(14, usize::MAX));
        assert_eq!(out.matches("'obsolete store_first'").count(), 3);
        assert_eq!(store.stats().instances, 0);
        assert_eq!(store.stats().expired_instances, 1);
        record(&f, &mut store, "a", 20, 1, 0, 20);
        let again = text(&store.emit(21, usize::MAX));
        assert_eq!(again.matches("CHART ").count(), 3);
        assert!(!again.contains("obsolete"));
        assert_eq!(again.matches("CLABEL_COMMIT").count(), 3);
        validate_chart_stream(format!("{out}{again}").as_bytes()).unwrap();
    }

    #[test]
    fn instances_over_the_cap_share_the_overflow_instance() {
        let f = fixture(0, 3, Some(1));
        let mut store = Store::new(f.family.clone());
        record(&f, &mut store, "a", 10, 1, 0, 10);
        record(&f, &mut store, "b", 10, 2, 0, 10);
        record(&f, &mut store, "c", 10, 3, 0, 10);
        let stats = store.stats();
        assert_eq!(stats.instances, 2);
        assert_eq!(stats.overflowed_records, 5);
        let out = text(&store.emit(11, usize::MAX));
        assert!(out.contains("BEGIN svc_overflow.requests 1000000\nSET ok = 5\n"));
        assert!(out.contains("CLABEL 'overflow' 'true' 1\n"));
        // Once "a" and the overflow instance expire, "b" gets its own charts.
        let expired = text(&store.emit(14, usize::MAX));
        assert_eq!(store.stats().instances, 0);
        record(&f, &mut store, "b", 20, 1, 0, 20);
        let b = text(&store.emit(21, usize::MAX));
        assert!(b.contains("CLABEL 'service.name' 'b' 1"));
        validate_chart_stream(format!("{out}{expired}{b}").as_bytes()).unwrap();
    }

    #[test]
    fn instances_without_records_are_not_created() {
        let f = fixture(0, 3, Some(1));
        let mut store = Store::new(f.family.clone());
        let mut batch = Batch::new(&f.family);
        batch.instance(labels("empty"));
        store.merge(batch, 10);
        assert_eq!(store.stats().instances, 0);
        // The cap is still free for an instance with records.
        record(&f, &mut store, "a", 10, 1, 0, 10);
        assert_eq!(store.stats().overflowed_records, 0);
        let out = text(&store.emit(11, usize::MAX));
        assert!(out.contains("CLABEL 'service.name' 'a' 1"));
        assert!(!out.contains("'empty'"));
    }

    #[test]
    fn a_clock_jump_renders_a_bounded_backlog_and_keeps_every_record() {
        let f = fixture(10, 1_000_000, None);
        let mut store = Store::new(f.family.clone());
        record(&f, &mut store, "a", 10, 1, 0, 10);
        record(&f, &mut store, "a", 100_000, 2, 0, 100_000);
        let window = store.instances.values().next().unwrap().slots.len() as u64;
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
    fn chunks_hold_whole_blocks_under_the_budget() {
        let f = fixture(0, 900, None);
        let mut store = Store::new(f.family.clone());
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
        let instance = new_instance(&f.family.0, labels, false);
        assert!(instance.chart_ids[0].starts_with("svc_"));
        assert!(netdata_plugin_protocol::is_valid_chart_id(
            &instance.chart_ids[0]
        ));
    }

    #[test]
    fn weights_scale_counts_and_histograms() {
        let f = fixture(0, 900, None);
        let mut store = Store::new(f.family.clone());
        let mut batch = Batch::new(&f.family);
        let inst = batch.instance(labels("a"));
        batch
            .record(inst, 10)
            .add(f.requests, 0, 2.0)
            .observe(f.duration, 0.25, 2.0);
        batch
            .record(inst, 10)
            .add(f.requests, 1, 1.0)
            .observe(f.duration, 2.5, 1.0);
        store.merge(batch, 10);
        let out = text(&store.emit(11, usize::MAX));
        assert!(out.contains("SET ok = 2\nSET error = 1\n"));
        assert!(out.contains("SET avg = 1\nSET max = 2.5\n"));
        assert!(out.contains("SET 0.1 = 0\nSET 1 = 2\nSET +Inf = 1\n"));
    }
}
