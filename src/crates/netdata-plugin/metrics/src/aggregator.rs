//! The aggregator: entities, the open windows, the cardinality limit,
//! expiry and emitting.
//!
//! The aggregator keeps every window from `oldest_open` (the oldest window
//! not yet emitted) to the current one open; each entity holds those windows
//! as rows and folds them forward after a stall (`bound_backlog`). Entities
//! are created on their first event, with or without measurements, chart
//! instances are defined on an entity's first emitted window, and an expired
//! entity is dropped with its rows and cumulative baselines.

use crate::batch::Batch;
use crate::chart::{EntityKind, chart_ids};
use crate::emit::{Chunker, write_definition, write_obsolete, write_update};
use crate::metric_set::{Labels, MetricSet, MetricSetInner};
use std::collections::hash_map::Entry;
use std::collections::{HashMap, VecDeque};

/// Most windows past the allowed lateness that one [`Aggregator::emit`] emits
/// after a stall or a forward clock step; older pending windows are folded
/// into the oldest one emitted, so no event is lost and memory stays bounded.
pub(crate) const MAX_BACKLOG_WINDOWS: u64 = 60;

/// Counts since the aggregator was created; `entities` is the current number.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct Stats {
    /// Entities currently held, the overflow entity included.
    pub entities: usize,
    /// Events counted in the overflow entity because of the cardinality
    /// limit.
    pub overflowed_events: u64,
    /// Events older than the oldest open window, counted in that window.
    pub late_events: u64,
    /// Events newer than the current window, counted in the current window.
    pub future_events: u64,
    /// Entities whose chart instances were marked obsolete after the expiry
    /// time.
    pub expired_entities: u64,
}

struct Entity {
    labels: Labels,
    /// The chart id of each of the entity's chart instances, in template
    /// order.
    chart_ids: Vec<String>,
    defined: bool,
    /// Earliest window with events; nothing is emitted before it.
    first_window: u64,
    /// Latest window with events; drives expiry.
    last_window: u64,
    /// The row of each open window from `oldest_open` onwards, oldest first;
    /// missing ones are empty.
    open_rows: VecDeque<Box<[f64]>>,
    /// The last emitted total of each cumulative column, in set order; NaN
    /// until the first one.
    previous_totals: Box<[f64]>,
}

/// Collects the batches of one [`MetricSet`] and emits each window, once its
/// allowed lateness has passed, as chart-protocol text.
///
/// The aggregator reads no clock and does no I/O. Pass the current Unix time
/// to [`Aggregator::merge`] and [`Aggregator::emit`], call `emit` at least
/// once per window, and write the bytes it returns to the agent. Share it
/// behind a lock.
///
/// # When a window is emitted
///
/// The *current* window is the one `now` falls in. A window is *open*, taking
/// events, until it is emitted: the first `emit` at or after its end plus the
/// [`Settings::allowed_lateness`](crate::Settings::allowed_lateness) emits it,
/// once. (After a stall, a window may instead be folded into a later one; see
/// below.)
///
/// Apart from a stall, only `emit` closes windows: a batch merged after the
/// `emit` that emitted its windows counts as late. A plugin that merges from
/// other threads, or some time after its events, needs an allowed lateness
/// that covers that delay.
///
/// With a 1 s collection interval and an allowed lateness of 10 s, the window
/// `[100, 101)` is emitted by the first `emit` at or after second 111. Until
/// then, events of second 100 still count in it. The agent stores its point at
/// second 101, the window's end, however late it is sent.
///
/// # Events outside the open windows
///
/// Every event is counted exactly once:
/// - one older than the oldest open window counts in that window
///   ([`Stats::late_events`]);
/// - one from the future counts in the current window
///   ([`Stats::future_events`]);
/// - after a stall (`emit` not called for a while, or the clock jumping
///   ahead), one `emit` emits at most 60 windows past the allowed lateness.
///   Older windows are not lost: their counts are added into the first
///   window emitted, which then shows them as one spike.
///
/// Gauge values and cumulative totals moved into another window do not keep
/// their event-time order:
/// - when a batch's late events from several windows land in one window, the
///   value kept comes from the window the batch first recorded into last (of
///   those that have a value), not from the latest event time;
/// - an older total merged after a newer one was emitted reads as a reset
///   when it is lower: the chart shows its whole value as one increase.
///
/// # Entities
///
/// - An entity starts with its first event. Its chart instances are
///   defined, with its labels, before their first update.
/// - After `expiry` without events its chart instances are marked obsolete
///   and the aggregator forgets it; if it returns, its chart instances are
///   defined again.
/// - Beyond the cardinality limit, events go to the overflow entity (see
///   [`CardinalityLimit`](crate::CardinalityLimit);
///   [`Stats::overflowed_events`]).
pub struct Aggregator {
    set: MetricSet,
    entities: HashMap<Labels, Entity>,
    overflow_entity: Option<Entity>,
    /// Oldest window not yet emitted; set on first use.
    oldest_open: Option<u64>,
    stats: Stats,
}

impl Aggregator {
    pub fn new(set: &MetricSet) -> Self {
        Self {
            set: set.clone(),
            entities: HashMap::new(),
            overflow_entity: None,
            oldest_open: None,
            stats: Stats::default(),
        }
    }

    pub fn stats(&self) -> Stats {
        Stats {
            entities: self.entities.len() + usize::from(self.overflow_entity.is_some()),
            ..self.stats
        }
    }

    /// The oldest window not yet emitted, as of `now`: set on first use, and
    /// moved forward, folding the windows it passes, when it falls more than
    /// [`MAX_BACKLOG_WINDOWS`] behind the allowed lateness.
    fn bound_backlog(&mut self, now: u64) -> u64 {
        let set = &*self.set.0;
        let now_window = now / set.interval_secs;
        let oldest_open = *self
            .oldest_open
            .get_or_insert(now_window.saturating_sub(set.allowed_lateness_windows));
        let oldest_wanted =
            now_window.saturating_sub(set.allowed_lateness_windows + MAX_BACKLOG_WINDOWS);
        if oldest_open >= oldest_wanted {
            return oldest_open;
        }
        let skipped = oldest_wanted - oldest_open;
        for entity in self
            .entities
            .values_mut()
            .chain(self.overflow_entity.as_mut())
        {
            entity.fold_front(set, skipped, oldest_wanted);
        }
        self.oldest_open = Some(oldest_wanted);
        oldest_wanted
    }

    /// Merge a batch. `now`, the current Unix time, picks the current window:
    /// newer events count in it ([`Stats::future_events`]). Merge before an
    /// `emit` with the same `now`: an event whose window that `emit` emits
    /// would otherwise count as late, in the next open window.
    ///
    /// # Panics
    ///
    /// If the batch is of another metric set.
    pub fn merge(&mut self, batch: Batch, now: u64) {
        assert!(
            std::sync::Arc::ptr_eq(&batch.set().0, &self.set.0),
            "batch of another metric set"
        );
        let oldest_open = self.bound_backlog(now);
        let set = &*self.set.0;
        let newest = (now / set.interval_secs).max(oldest_open);
        for (labels, entity_rows) in batch.entities {
            if entity_rows.rows.is_empty() {
                continue;
            }
            let events: u64 = entity_rows.rows.values().map(|row| row.events).sum();
            let full = set
                .cardinality_limit
                .as_ref()
                .filter(|limit| self.entities.len() >= limit.max_entities);
            let entity = match (self.entities.entry(labels), full) {
                (Entry::Occupied(entry), _) => entry.into_mut(),
                (Entry::Vacant(entry), None) => {
                    let entity = Entity::new(set, entry.key().clone(), EntityKind::Own);
                    entry.insert(entity)
                }
                (Entry::Vacant(_), Some(limit)) => {
                    self.stats.overflowed_events += events;
                    self.overflow_entity.get_or_insert_with(|| {
                        Entity::new(set, limit.overflow_labels.clone(), EntityKind::Overflow)
                    })
                }
            };
            for (window, batch_row) in entity_rows.rows {
                let window = if window < oldest_open {
                    self.stats.late_events += batch_row.events;
                    oldest_open
                } else if window > newest {
                    self.stats.future_events += batch_row.events;
                    newest
                } else {
                    window
                };
                entity.merge_window(set, oldest_open, window, &batch_row.cells);
            }
        }
    }

    /// Emit every window whose allowed lateness has passed by `now` (Unix
    /// seconds): the definitions of new entities' chart instances, one update
    /// per chart instance and window, and the obsolete marks of expired
    /// entities.
    ///
    /// The text is complete chart-protocol text, to write to the agent as is
    /// (a plugin's stdout). It comes in chunks of at most `max_chunk` bytes,
    /// split only between whole definitions, updates and obsolete marks, so
    /// each chunk can be written on its own, for example as one message over
    /// a size-limited channel; one larger than `max_chunk` gets a chunk of
    /// its own. Pass that channel's limit, or `usize::MAX` when writing
    /// straight to stdout.
    pub fn emit(&mut self, now: u64, max_chunk: usize) -> Vec<Vec<u8>> {
        let mut out = Chunker::new(max_chunk);
        let mut oldest_open = self.bound_backlog(now);
        let set = &*self.set.0;
        let now_window = now / set.interval_secs;
        while oldest_open + set.allowed_lateness_windows < now_window {
            let window = oldest_open;
            let stats = &mut self.stats;
            self.entities.retain(|_, entity| {
                entity.emit_window(set, window, &mut out);
                let expired = entity.expire_if_idle(set, window, &mut out);
                stats.expired_entities += u64::from(expired);
                !expired
            });
            let expired = self.overflow_entity.take_if(|entity| {
                entity.emit_window(set, window, &mut out);
                entity.expire_if_idle(set, window, &mut out)
            });
            if expired.is_some() {
                self.stats.expired_entities += 1;
            }
            oldest_open += 1;
        }
        self.oldest_open = Some(oldest_open);
        out.finish()
    }
}

impl Entity {
    fn new(set: &MetricSetInner, labels: Labels, kind: EntityKind) -> Self {
        Self {
            chart_ids: chart_ids(&set.chart_templates, &labels, kind),
            labels,
            defined: false,
            first_window: u64::MAX,
            last_window: 0,
            open_rows: VecDeque::new(),
            previous_totals: vec![f64::NAN; set.row_layout.cumulative_columns().len()]
                .into_boxed_slice(),
        }
    }

    /// Merge one batch row into this entity's row of `window`, an open window
    /// from `oldest_open` onwards.
    fn merge_window(&mut self, set: &MetricSetInner, oldest_open: u64, window: u64, row: &[f64]) {
        let index = (window - oldest_open) as usize;
        while self.open_rows.len() <= index {
            self.open_rows.push_back(set.row_layout.empty_row());
        }
        set.row_layout.combine(&mut self.open_rows[index], row);
        self.first_window = self.first_window.min(window);
        self.last_window = self.last_window.max(window);
    }

    /// Fold the first `skipped` open windows into the window that becomes the
    /// oldest open one, `new_first`.
    fn fold_front(&mut self, set: &MetricSetInner, skipped: u64, new_first: u64) {
        let take = (skipped as usize).min(self.open_rows.len());
        if take == 0 {
            return;
        }
        let mut folded = set.row_layout.empty_row();
        for row in self.open_rows.drain(..take) {
            set.row_layout.combine(&mut folded, &row);
        }
        match self.open_rows.front_mut() {
            Some(front) => set.row_layout.combine(front, &folded),
            None => self.open_rows.push_back(folded),
        }
        self.first_window = self.first_window.max(new_first);
    }

    /// Emit `window`, the oldest open one, and drop its row; nothing
    /// is written before the entity's first event.
    fn emit_window(&mut self, set: &MetricSetInner, window: u64, out: &mut Chunker) {
        let row = self.open_rows.pop_front();
        if window < self.first_window {
            return;
        }
        if !self.defined {
            for (template, id) in set.chart_templates.iter().zip(&self.chart_ids) {
                out.block(|block| {
                    write_definition(block, template, id, &self.labels, set.interval_secs)
                });
            }
            self.defined = true;
        }
        let mut row = row.unwrap_or_else(|| set.row_layout.empty_row());
        self.totals_to_increases(set, &mut row);
        for (template, id) in set.chart_templates.iter().zip(&self.chart_ids) {
            out.block(|block| write_update(block, template, id, &row, window, set.interval_secs));
        }
    }

    /// When the entity has had no events for the expiry time as of `window`,
    /// mark its chart instances obsolete and return true; the caller drops
    /// it.
    fn expire_if_idle(&self, set: &MetricSetInner, window: u64, out: &mut Chunker) -> bool {
        if !self.defined || window < self.last_window + set.expiry_windows {
            return false;
        }
        for (template, id) in set.chart_templates.iter().zip(&self.chart_ids) {
            out.block(|block| write_obsolete(block, template, id, set.interval_secs));
        }
        true
    }

    /// Replace the totals in `row` with their increase since the entity's
    /// previous total, and remember the new totals.
    fn totals_to_increases(&mut self, set: &MetricSetInner, row: &mut [f64]) {
        for (&column, previous) in set
            .row_layout
            .cumulative_columns()
            .iter()
            .zip(self.previous_totals.iter_mut())
        {
            let total = row[column];
            row[column] = if total.is_nan() {
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
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{
        CardinalityLimit, ChartDescriptor, ChartType, Counter, Histogram, MetricSetBuilder,
        Settings,
    };
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

    fn fixture(allowed_lateness: u64, expiry: u64, max: Option<usize>) -> Fixture {
        let cardinality_limit = max.map(|max_entities| {
            let mut overflow_labels = Labels::new();
            overflow_labels.insert("overflow", "true");
            CardinalityLimit {
                max_entities,
                overflow_labels,
            }
        });
        let mut builder = MetricSetBuilder::new(Settings {
            allowed_lateness: Duration::from_secs(allowed_lateness),
            expiry: Duration::from_secs(expiry),
            cardinality_limit,
            ..Settings::default()
        });
        let requests = builder.counter::<Outcome>(ChartDescriptor {
            title: "Requests",
            units: "requests/s",
            family: "requests",
            context: "svc.requests",
            chart_type: ChartType::Stacked,
            priority: 1,
        });
        let duration = builder.histogram(
            BOUNDS,
            ChartDescriptor {
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
            set: builder.build(),
            requests,
            duration,
        }
    }

    fn labels(name: &str) -> Labels {
        let mut labels = Labels::new();
        labels.insert("service.name", name);
        labels
    }

    fn text(chunks: &[Vec<u8>]) -> String {
        String::from_utf8(chunks.concat()).unwrap()
    }

    /// Records `count` requests of `name` at `time`, the first `errors` of them
    /// failed, each taking half a second.
    fn record(
        f: &Fixture,
        aggregator: &mut Aggregator,
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
        aggregator.merge(batch, now);
    }

    #[test]
    fn defines_once_then_updates_each_window() {
        let f = fixture(0, 900, None);
        let mut aggregator = Aggregator::new(&f.set);
        record(&f, &mut aggregator, "checkout", 100, 3, 1, 100);
        let first = text(&aggregator.emit(101, usize::MAX));
        let h = format!("{:016x}", labels("checkout").stable_hash());
        assert_eq!(
            first,
            format!(
                "CHART svc.requests_{h} '' 'Requests' 'requests/s' 'requests' 'svc.requests' stacked 1 1 'store_first'\n\
                 CLABEL 'service.name' 'checkout' 1\nCLABEL_COMMIT\n\
                 DIMENSION ok 'ok' absolute 1 1 'type=float'\n\
                 DIMENSION error 'error' absolute 1 1 'type=float'\n\
                 CHART svc.duration_avg_{h} '' 'Duration average' 'seconds' 'requests' 'svc.duration_avg' line 2 1 \
                 'store_first'\n\
                 CLABEL 'service.name' 'checkout' 1\nCLABEL_COMMIT\n\
                 DIMENSION avg 'avg' absolute 1 1 'type=float'\n\
                 CHART svc.duration_max_{h} '' 'Duration maximum' 'seconds' 'requests' 'svc.duration_max' line 3 1 \
                 'store_first'\n\
                 CLABEL 'service.name' 'checkout' 1\nCLABEL_COMMIT\n\
                 DIMENSION max 'max' absolute 1 1 'type=float'\n\
                 CHART svc.duration_histogram_{h} '' 'Duration histogram' 'requests/s' 'requests' \
                 'svc.duration_histogram' heatmap 4 1 'store_first'\n\
                 CLABEL 'service.name' 'checkout' 1\nCLABEL_COMMIT\n\
                 DIMENSION 0.1 '0.1' absolute 1 1 'type=float'\n\
                 DIMENSION 1 '1' absolute 1 1 'type=float'\n\
                 DIMENSION +Inf '+Inf' absolute 1 1 'type=float'\n\
                 BEGIN svc.requests_{h} 1000000\nSET ok = 2\nSET error = 1\nEND 101 0 1\n\
                 BEGIN svc.duration_avg_{h} 1000000\nSET avg = 0.5\nEND 101 0 1\n\
                 BEGIN svc.duration_max_{h} 1000000\nSET max = 0.5\nEND 101 0 1\n\
                 BEGIN svc.duration_histogram_{h} 1000000\nSET 0.1 = 0\nSET 1 = 3\nSET +Inf = 0\nEND 101 0 1\n"
            )
        );
        // The next window: zero rates, no average or maximum, no new
        // definition.
        let second = text(&aggregator.emit(102, usize::MAX));
        assert_eq!(
            second,
            format!(
                "BEGIN svc.requests_{h} 1000000\nSET ok = 0\nSET error = 0\nEND 102 0 1\n\
                 BEGIN svc.duration_avg_{h} 1000000\nSET avg =\nEND 102 0 1\n\
                 BEGIN svc.duration_max_{h} 1000000\nSET max =\nEND 102 0 1\n\
                 BEGIN svc.duration_histogram_{h} 1000000\nSET 0.1 = 0\nSET 1 = 0\nSET +Inf = 0\nEND 102 0 1\n"
            )
        );
        validate_chart_stream(format!("{first}{second}").as_bytes()).unwrap();
        // Nothing is emitted twice.
        assert!(aggregator.emit(102, usize::MAX).is_empty());
    }

    #[test]
    fn allowed_lateness_keeps_windows_open_and_clamps_exactly_once() {
        let f = fixture(10, 900, None);
        let mut aggregator = Aggregator::new(&f.set);
        // The oldest open window is 990 (now 1000, allowed lateness 10).
        record(&f, &mut aggregator, "a", 995, 1, 0, 1000);
        // Too late: counted in the oldest open window.
        record(&f, &mut aggregator, "a", 100, 2, 0, 1000);
        // From the future: counted in the current window.
        record(&f, &mut aggregator, "a", 5000, 4, 0, 1000);
        assert_eq!(aggregator.stats().late_events, 2);
        assert_eq!(aggregator.stats().future_events, 4);
        // Window 999 is not emitted before its end plus the allowed lateness,
        // 1010.
        let out = text(&aggregator.emit(1009, usize::MAX));
        validate_chart_stream(out.as_bytes()).unwrap();
        let ok_values = |out: &str| -> Vec<f64> {
            out.lines()
                .filter_map(|l| l.strip_prefix("SET ok = "))
                .map(|v| v.parse().unwrap())
                .collect()
        };
        // Windows 990..=998: the clamped 2 land in 990, the event at 995.
        let emitted = ok_values(&out);
        assert_eq!(emitted.len(), 9);
        assert_eq!(emitted[0], 2.0);
        assert_eq!(emitted[5], 1.0);
        let rest = text(&aggregator.emit(1011, usize::MAX));
        // Windows 999 and 1000: the 4 future events land in window 1000.
        assert_eq!(ok_values(&rest), vec![0.0, 4.0]);
        validate_chart_stream(format!("{out}{rest}").as_bytes()).unwrap();
    }

    #[test]
    fn expired_entities_are_marked_obsolete_and_revived() {
        let f = fixture(0, 3, None);
        let mut aggregator = Aggregator::new(&f.set);
        record(&f, &mut aggregator, "a", 10, 1, 0, 10);
        let mut out = text(&aggregator.emit(13, usize::MAX));
        assert!(!out.contains("obsolete"));
        out += &text(&aggregator.emit(14, usize::MAX));
        assert_eq!(out.matches("'obsolete store_first'").count(), 4);
        assert_eq!(aggregator.stats().entities, 0);
        assert_eq!(aggregator.stats().expired_entities, 1);
        record(&f, &mut aggregator, "a", 20, 1, 0, 20);
        let again = text(&aggregator.emit(21, usize::MAX));
        assert_eq!(again.matches("CHART ").count(), 4);
        assert!(!again.contains("obsolete"));
        assert_eq!(again.matches("CLABEL_COMMIT").count(), 4);
        validate_chart_stream(format!("{out}{again}").as_bytes()).unwrap();
    }

    #[test]
    fn entities_over_the_limit_share_the_overflow_entity() {
        let f = fixture(0, 3, Some(1));
        let mut aggregator = Aggregator::new(&f.set);
        record(&f, &mut aggregator, "a", 10, 1, 0, 10);
        record(&f, &mut aggregator, "b", 10, 2, 0, 10);
        record(&f, &mut aggregator, "c", 10, 3, 0, 10);
        let stats = aggregator.stats();
        assert_eq!(stats.entities, 2);
        assert_eq!(stats.overflowed_events, 5);
        let out = text(&aggregator.emit(11, usize::MAX));
        assert!(out.contains("BEGIN svc.requests_overflow 1000000\nSET ok = 5\n"));
        assert!(out.contains("CLABEL 'overflow' 'true' 1\n"));
        // Once "a" and the overflow entity expire, "b" gets its own charts.
        let expired = text(&aggregator.emit(14, usize::MAX));
        assert_eq!(aggregator.stats().entities, 0);
        record(&f, &mut aggregator, "b", 20, 1, 0, 20);
        let b = text(&aggregator.emit(21, usize::MAX));
        assert!(b.contains("CLABEL 'service.name' 'b' 1"));
        validate_chart_stream(format!("{out}{expired}{b}").as_bytes()).unwrap();
    }

    #[test]
    fn entities_without_events_are_not_created() {
        let f = fixture(0, 3, Some(1));
        let mut aggregator = Aggregator::new(&f.set);
        let mut batch = Batch::new(&f.set);
        batch.entity(labels("empty"));
        aggregator.merge(batch, 10);
        assert_eq!(aggregator.stats().entities, 0);
        // The limit still has room for an entity with events.
        record(&f, &mut aggregator, "a", 10, 1, 0, 10);
        assert_eq!(aggregator.stats().overflowed_events, 0);
        let out = text(&aggregator.emit(11, usize::MAX));
        assert!(out.contains("CLABEL 'service.name' 'a' 1"));
        assert!(!out.contains("'empty'"));
    }

    #[test]
    fn a_clock_jump_renders_a_bounded_backlog_and_keeps_every_event() {
        let f = fixture(10, 1_000_000, None);
        let mut aggregator = Aggregator::new(&f.set);
        record(&f, &mut aggregator, "a", 10, 1, 0, 10);
        record(&f, &mut aggregator, "a", 100_000, 2, 0, 100_000);
        let open = aggregator.entities.values().next().unwrap().open_rows.len() as u64;
        assert!(open <= 10 + MAX_BACKLOG_WINDOWS + 1, "{open}");
        let out = text(&aggregator.emit(100_011, usize::MAX));
        validate_chart_stream(out.as_bytes()).unwrap();
        let updates = out.matches("BEGIN ").count() as u64 / 4;
        assert!(updates <= MAX_BACKLOG_WINDOWS + 11, "{updates}");
        let ok: f64 = out
            .lines()
            .filter_map(|l| l.strip_prefix("SET ok = "))
            .map(|v| v.parse::<f64>().unwrap())
            .sum();
        assert_eq!(ok, 3.0);
    }

    #[test]
    fn events_spread_over_many_seconds_land_in_their_windows() {
        let f = fixture(10, 900, None);
        let mut aggregator = Aggregator::new(&f.set);
        let mut batch = Batch::new(&f.set);
        let mut entity = batch.entity(labels("a"));
        for time in [995, 996, 995, 997, 996, 999] {
            entity.record(time).add(f.requests, Outcome::Ok, 1.0);
        }
        aggregator.merge(batch, 1000);
        let out = text(&aggregator.emit(1011, usize::MAX));
        let ok: Vec<&str> = out
            .lines()
            .filter_map(|l| l.strip_prefix("SET ok = "))
            .collect();
        assert_eq!(ok, ["2", "2", "1", "0", "1", "0"]);
    }

    #[test]
    fn chunks_hold_whole_blocks_under_the_budget() {
        let f = fixture(0, 900, None);
        let mut aggregator = Aggregator::new(&f.set);
        for name in ["a", "b", "c", "d"] {
            record(&f, &mut aggregator, name, 10, 1, 0, 10);
        }
        let chunks = aggregator.emit(12, 400);
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
        labels.insert("empty", "");
        labels.insert("blank", " \t\n");
        labels.insert("long", &"é".repeat(500));
        assert_eq!(labels.0.len(), 1);
        assert_eq!(labels.0[0].1.len(), 800);
        let f = fixture(0, 900, None);
        let entity = Entity::new(&f.set.0, labels, EntityKind::Own);
        assert!(entity.chart_ids[0].starts_with("svc.requests_"));
        assert!(netdata_plugin_protocol::is_valid_chart_id(
            &entity.chart_ids[0]
        ));
    }

    #[test]
    fn insert_order_does_not_change_the_entity_or_its_chart_ids() {
        let mut ab = Labels::new();
        ab.insert("a", "1");
        ab.insert("b", "2");
        let mut ba = Labels::new();
        ba.insert("b", "2");
        ba.insert("a", "1");
        assert_eq!(ab, ba);
        let f = fixture(0, 900, None);
        let ids = |labels: Labels| Entity::new(&f.set.0, labels, EntityKind::Own).chart_ids;
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
        let mut builder = MetricSetBuilder::new(Settings {
            expiry: Duration::from_secs(3),
            ..Settings::default()
        });
        let totals = builder.cumulative_counter::<Events>(ChartDescriptor {
            title: "Events",
            units: "events/s",
            family: "events",
            context: "src.events",
            chart_type: ChartType::Line,
            priority: 1,
        });
        let set = builder.build();
        let mut aggregator = Aggregator::new(&set);
        let set_total = |aggregator: &mut Aggregator, time: u64, total: Option<f64>| {
            let mut batch = Batch::new(&set);
            let mut entity = batch.entity(Labels::new());
            let mut measurements = entity.record(time);
            if let Some(total) = total {
                measurements.set_total(totals, Events::Events, total);
            }
            aggregator.merge(batch, time);
        };
        // Baseline, two totals in one window (the last wins), no new total, a
        // reset, then a return after expiry with a new baseline.
        set_total(&mut aggregator, 10, Some(100.0));
        set_total(&mut aggregator, 11, Some(103.0));
        set_total(&mut aggregator, 11, Some(107.0));
        set_total(&mut aggregator, 12, None);
        set_total(&mut aggregator, 13, Some(5.0));
        let mut out = text(&aggregator.emit(14, usize::MAX));
        out += &text(&aggregator.emit(18, usize::MAX));
        set_total(&mut aggregator, 20, Some(50.0));
        set_total(&mut aggregator, 21, Some(52.0));
        out += &text(&aggregator.emit(22, usize::MAX));
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
        impl crate::Dimension for Broken {
            const IDS: &'static [&'static str] = &["a", "b"];
            fn index(self) -> usize {
                2
            }
        }
        let mut builder = MetricSetBuilder::new(Settings::default());
        let counter = builder.counter::<Broken>(ChartDescriptor {
            title: "C",
            units: "events/s",
            family: "c",
            context: "x.c",
            chart_type: ChartType::Line,
            priority: 1,
        });
        let set = builder.build();
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
    fn a_batch_without_events_is_empty() {
        let f = fixture(0, 900, None);
        let mut batch = Batch::new(&f.set);
        batch.entity(labels("a"));
        assert!(batch.is_empty());
        batch.entity(labels("a")).record(10);
        assert!(!batch.is_empty());
    }

    #[test]
    fn non_finite_measurements_are_ignored() {
        crate::dimensions! {
            enum Queued {
                Jobs = "jobs",
            }
        }
        crate::dimensions! {
            enum Events {
                Events = "events",
            }
        }
        let mut builder = MetricSetBuilder::new(Settings::default());
        let descriptor = |context| ChartDescriptor {
            title: "T",
            units: "u",
            family: "f",
            context,
            chart_type: ChartType::Line,
            priority: 1,
        };
        let requests = builder.counter::<Outcome>(descriptor("svc.requests"));
        let queued = builder.gauge::<Queued>(descriptor("svc.queued"));
        let totals = builder.cumulative_counter::<Events>(descriptor("svc.events"));
        let duration = builder.histogram(BOUNDS, descriptor("svc.duration"), "requests/s");
        let set = builder.build();
        let mut aggregator = Aggregator::new(&set);
        let mut batch = Batch::new(&set);
        let mut entity = batch.entity(Labels::new());
        entity
            .record(10)
            .add(requests, Outcome::Ok, 1.0)
            .add(requests, Outcome::Ok, f64::NAN)
            .add(requests, Outcome::Ok, f64::INFINITY)
            .set_value(queued, Queued::Jobs, 5.0)
            .set_value(queued, Queued::Jobs, f64::NAN)
            .set_total(totals, Events::Events, 10.0)
            .observe(duration, 0.5, 1.0)
            .observe(duration, 0.5, f64::NAN);
        aggregator.merge(batch, 10);
        let mut batch = Batch::new(&set);
        batch
            .entity(Labels::new())
            .record(11)
            .set_total(totals, Events::Events, 13.0)
            .set_total(totals, Events::Events, f64::NAN);
        aggregator.merge(batch, 11);
        let out = text(&aggregator.emit(13, usize::MAX));
        assert!(out.contains("SET ok = 1\nSET error = 0\n"));
        assert!(out.contains("SET jobs = 5\n"));
        assert!(out.contains("SET events = 3\n"));
        assert!(out.contains("SET avg = 0.5\nEND"));
        assert!(out.contains("SET max = 0.5\nEND"));
        assert!(out.contains("SET 0.1 = 0\nSET 1 = 1\nSET +Inf = 0\n"));
    }

    #[test]
    fn weights_scale_counts_and_histograms() {
        let f = fixture(0, 900, None);
        let mut aggregator = Aggregator::new(&f.set);
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
        aggregator.merge(batch, 10);
        let out = text(&aggregator.emit(11, usize::MAX));
        assert!(out.contains("SET ok = 2\nSET error = 1\n"));
        assert!(out.contains("SET avg = 1\nEND"));
        assert!(out.contains("SET max = 2.5\nEND"));
        assert!(out.contains("SET 0.1 = 0\nSET 1 = 2\nSET +Inf = 1\n"));
    }
}
