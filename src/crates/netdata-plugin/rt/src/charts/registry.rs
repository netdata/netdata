//! The chart registry background task: it schedules the plugin's registered
//! chart samplers and emits their pluginsd chart commands (CHART/DIMENSION per
//! chart once, BEGIN/SET/END every tick) through the plugin's shared writer.
//!
//! Lifecycle: `PluginRuntime` creates the registry lazily on the first chart
//! registration (`register_chart` / `register_instanced_chart`, crate root)
//! and, when the runtime starts, spawns [`ChartRegistry::run`] as the
//! chart-registry task and cancels it on shutdown through the token from
//! [`ChartRegistry::cancellation_token`]. `run()` consumes the registry and
//! splits the samplers into batches of up to 1000, one task per batch; each
//! batch task owns a tick timer and a reusable buffer, samples its charts
//! serially, and takes the writer lock once per tick. The batched interval
//! behavior — a batch ticks at its first sampler's interval — is documented on
//! [`ChartRegistry::run`] and [`ChartRegistry::register_chart`].

use super::chart_trait::{InstancedChart, NetdataChart};
use super::handle::ChartHandle;
use super::tracker::TrackedChart;
use super::writer::ChartWriter;
use async_trait::async_trait;
use bytes::BytesMut;
use netdata_plugin_protocol::MessageWriter;
use parking_lot::RwLock;
use std::sync::Arc;
use std::time::Duration;
use tokio::io::AsyncWrite;
use tokio::sync::Mutex;
use tokio::task::JoinSet;
use tokio_util::sync::CancellationToken;

/// Registry of chart samplers, driven by the background task [`Self::run`].
///
/// One instance per plugin runtime. Charts must be registered before
/// [`Self::run`]: registration pushes one boxed `ChartSampler` per chart (in
/// registration order), and `run` drains them, so a registry cannot be reused.
/// The registry also holds the cancellation token stopping the batch tasks and
/// a clone of the shared writer mutex all batch tasks write through.
pub struct ChartRegistry<W>
where
    W: AsyncWrite + Unpin + Send + 'static,
{
    samplers: Vec<Box<dyn ChartSampler>>,
    cancellation: CancellationToken,
    writer: Arc<Mutex<MessageWriter<W>>>,
}

impl<W> ChartRegistry<W>
where
    W: AsyncWrite + Unpin + Send + 'static,
{
    /// Create a registry that writes through `writer` — the mutex-protected
    /// outbound writer shared with the rest of the plugin's protocol output.
    pub fn new(writer: Arc<Mutex<MessageWriter<W>>>) -> Self {
        Self {
            samplers: Vec::new(),
            cancellation: CancellationToken::new(),
            writer,
        }
    }

    /// Register a chart and get the handle that updates its values.
    ///
    /// The chart's CHART/DIMENSION definition is emitted once on the first
    /// tick after [`Self::run`] starts, then BEGIN/SET/END on every batch
    /// tick; updates made through the handle are emitted from the next tick
    /// on.
    ///
    /// `interval` is the chart's declared sample period and the `update_every`
    /// of its BEGIN commands, but it only drives emission cadence when this
    /// chart is the first sampler of its batch: batches tick at the first
    /// sampler's interval (details in [`Self::run`]), so keep a registry on a
    /// single interval unless its chart count can cross batch boundaries.
    ///
    /// # Example
    ///
    /// ```ignore
    /// let handle = registry.register_chart(
    ///     CpuMetrics::default(),
    ///     Duration::from_secs(1),
    /// );
    ///
    /// // Update the chart from anywhere
    /// handle.update(|m| {
    ///     m.user = 42;
    ///     m.system = 13;
    /// });
    /// ```
    pub fn register_chart<T>(&mut self, initial: T, interval: Duration) -> ChartHandle<T>
    where
        T: NetdataChart + Default + PartialEq + Clone + Send + Sync + 'static,
    {
        let handle = ChartHandle::new(initial.clone());

        let sampler = SingletonChartSampler {
            data: handle.clone(),
            tracker: TrackedChart::new(initial, interval),
            writer: ChartWriter::new(),
        };

        self.samplers.push(Box::new(sampler));
        handle
    }

    /// Register one concrete instance of an instanced chart (e.g. per-CPU).
    ///
    /// Identical to [`Self::register_chart`] except that the chart metadata is
    /// instantiated with the initial value's `instance_id()` at registration
    /// ([`TrackedChart::new_instanced`]), so each instance needs its own
    /// registration call. Interval behavior matches `register_chart`.
    pub fn register_instanced_chart<T>(&mut self, initial: T, interval: Duration) -> ChartHandle<T>
    where
        T: InstancedChart + Default + PartialEq + Send + Sync + 'static,
    {
        let handle = ChartHandle::new(initial.clone());

        let sampler = SingletonChartSampler {
            data: handle.clone(),
            tracker: TrackedChart::new_instanced(initial, interval),
            writer: ChartWriter::new(),
        };

        self.samplers.push(Box::new(sampler));
        handle
    }

    /// Get the token that stops the registry: cancelling it ends every batch
    /// task's tick loop, after which [`Self::run`] joins the tasks and
    /// returns. `PluginRuntime` cancels it when the plugin shuts down.
    pub fn cancellation_token(&self) -> CancellationToken {
        self.cancellation.clone()
    }

    /// Run the registry until cancelled, sampling the registered charts.
    ///
    /// Consumes the registry: the samplers registered so far are split into
    /// batches of up to 1000 by plain count, and one task is spawned per
    /// batch (batches run concurrently). Each batch task ticks on a
    /// `tokio::time::interval` — the first tick fires immediately, so chart
    /// definitions go out as soon as the task starts — samples its charts
    /// serially into one reusable buffer, and writes that buffer with a
    /// single acquisition of the shared writer lock.
    ///
    /// Scheduling is per batch, not per chart: the batch ticks at its FIRST
    /// sampler's interval, and the other samplers' registered intervals only
    /// shape their own BEGIN `update_every`. With fewer than 1000 charts a
    /// registry is a single batch, so every chart is sampled at the
    /// first-registered chart's interval — netflow-plugin registers its
    /// memory charts with a configurable interval (default 10s) after its
    /// 1-second charts, and they are sampled every second. A registered
    /// interval is honored only when its chart opens a batch, so keep a
    /// registry on one interval unless the chart count crosses batch
    /// boundaries. Missed ticks are caught up in a burst rather than skipped,
    /// so a stalled batch re-emits its charts once per missed tick on resume.
    ///
    /// Cancelling [`Self::cancellation_token`] stops the tick loops; `run`
    /// then joins the batch tasks and returns. Writer errors are discarded,
    /// so the only error `run` reports is a batch task panic.
    pub async fn run(mut self) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        const BATCH_SIZE: usize = 1000;
        let mut tasks = JoinSet::new();

        // Group samplers into batches to reduce writer lock contention
        let mut batch_samplers = Vec::new();
        let mut current_batch = Vec::new();

        for sampler in self.samplers.drain(..) {
            current_batch.push(sampler);
            if current_batch.len() >= BATCH_SIZE {
                batch_samplers.push(std::mem::take(&mut current_batch));
            }
        }

        if !current_batch.is_empty() {
            batch_samplers.push(current_batch);
        }

        for mut batch in batch_samplers {
            let token = self.cancellation.child_token();
            let writer = Arc::clone(&self.writer);

            // The batch ticks at the FIRST sampler's interval: every chart in
            // the batch is sampled on that cadence, whatever interval it was
            // registered with. A non-first sampler's interval still reaches
            // its BEGIN update_every, so its cadence can disagree with its
            // declared period. Batches are cut by count only, so a registered
            // interval is honored only when its chart opens a batch; the
            // unwrap_or fallback never fires (batches are never empty).
            let interval = batch
                .first()
                .map(|s| s.interval())
                .unwrap_or(Duration::from_secs(1));

            tasks.spawn(async move {
                // Reusable batch buffer: cleared each tick, grows past 512KB if needed.
                let mut batch_buffer = BytesMut::with_capacity(512 * 1024);
                let mut interval_timer = tokio::time::interval(interval);
                // Catch up missed ticks (Burst) rather than skipping them.
                interval_timer.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Burst);

                loop {
                    tokio::select! {
                        _ = token.cancelled() => break,
                        _ = interval_timer.tick() => {
                            batch_buffer.clear();

                            // One timestamp per tick, shared by every chart's END line this tick.
                            let collection_time = std::time::SystemTime::now();

                            for sampler in &mut batch {
                                sampler.sample_to_buffer(&mut batch_buffer, collection_time).await;
                            }

                            // Write errors are discarded: the batch keeps
                            // running and retries on the next tick.
                            if !batch_buffer.is_empty() {
                                let mut w = writer.lock().await;
                                let _ = w.write_raw(&batch_buffer).await;
                            }
                        }
                    }
                }
            });
        }

        // Join the batch tasks; a JoinError here is a batch task panic and fails run().
        while let Some(result) = tasks.join_next().await {
            result?;
        }

        Ok(())
    }
}

/// Type-erased view of one registered chart, boxed into a batch.
///
/// `SingletonChartSampler` is the only implementation. The batch task calls
/// `sample_to_buffer` once per tick in registration order and reads only the
/// first sampler's `interval` to set the tick period (see `run`).
#[async_trait]
trait ChartSampler: Send + Sync {
    /// Emit one tick of this chart: append its definition (once) and its
    /// BEGIN/SET/END update to `buffer`, stamped with `collection_time`.
    ///
    /// Async by signature only — no implementation awaits — so a batch tick's
    /// sampling is synchronous work between the timer and the writer lock.
    async fn sample_to_buffer(
        &mut self,
        buffer: &mut bytes::BytesMut,
        collection_time: std::time::SystemTime,
    );
    /// The chart's registered sample period. Only the first sampler of a
    /// batch drives the tick timer; see `run`.
    fn interval(&self) -> Duration;
}

/// One registered chart, sampled once per batch tick as a [`ChartSampler`].
///
/// Used for both singleton and instanced registrations — the name is
/// historical: instancing is resolved at registration time by
/// [`TrackedChart::new_instanced`], which bakes the instance id into the chart
/// metadata. `data` is the registry's own clone of the chart's handle, so
/// dropping the last plugin-side handle does not stop emission; `writer`
/// stages each tick's bytes before they are appended to the batch buffer.
struct SingletonChartSampler<T> {
    data: ChartHandle<T>,
    tracker: TrackedChart<T>,
    writer: ChartWriter,
}

#[async_trait]
impl<T> ChartSampler for SingletonChartSampler<T>
where
    T: NetdataChart + Default + PartialEq + Clone + Send + Sync,
{
    async fn sample_to_buffer(
        &mut self,
        buffer: &mut BytesMut,
        collection_time: std::time::SystemTime,
    ) {
        // One tick of this chart: read the handle's current value, then emit
        // the definition once and an update every tick — unconditionally, even
        // when nothing changed, so each sample interval carries a datapoint.
        // Change detection (TrackedChart::has_changed) is informational and
        // does not gate emission.
        let current = {
            let guard = self.data.read();
            (*guard).clone()
        };

        self.tracker.update(current);

        if !self.tracker.defined {
            self.tracker.emit_definition(&mut self.writer);
        }

        self.tracker.emit_update(&mut self.writer, collection_time);

        self.writer.append_to(buffer);
    }

    fn interval(&self) -> Duration {
        self.tracker.interval
    }
}
