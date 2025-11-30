//! Chart registry for managing and scheduling chart updates.

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

/// Registry for managing charts and their sampling schedules.
///
/// The registry maintains a collection of charts and spawns background tasks
/// to sample them at their configured intervals. Chart data is written through
/// the provided MessageWriter, coordinating with other plugin output.
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
    /// Create a new chart registry with a shared message writer
    pub fn new(writer: Arc<Mutex<MessageWriter<W>>>) -> Self {
        Self {
            samplers: Vec::new(),
            cancellation: CancellationToken::new(),
            writer,
        }
    }

    /// Register a chart and get a handle to update it.
    ///
    /// The chart will be sampled at the given interval and updates will be
    /// emitted through the message writer using the Netdata chart protocol.
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

    /// Register an instanced chart (for per-instance charts like per-CPU metrics).
    ///
    /// This is a specialized version of register_chart that properly handles
    /// template instantiation for charts that have instance identifiers.
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

    /// Get a cancellation token that can be used to stop the registry
    pub fn cancellation_token(&self) -> CancellationToken {
        self.cancellation.clone()
    }

    /// Run the registry, sampling all charts at their configured intervals.
    ///
    /// This method consumes the registry and runs until cancelled.
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

        // Don't forget the last partial batch
        if !current_batch.is_empty() {
            batch_samplers.push(current_batch);
        }

        for mut batch in batch_samplers {
            let token = self.cancellation.child_token();
            let writer = Arc::clone(&self.writer);

            // All samplers in a batch should have the same interval (typically 1 second)
            let interval = batch
                .first()
                .map(|s| s.interval())
                .unwrap_or(Duration::from_secs(1));

            tasks.spawn(async move {
                // Reusable buffer for entire batch (512KB should be enough for 1000 charts)
                let mut batch_buffer = BytesMut::with_capacity(512 * 1024);
                let mut interval_timer = tokio::time::interval(interval);
                // Use Burst to catch up if a tick is delayed, ensuring consistent emission rate
                interval_timer.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Burst);

                loop {
                    tokio::select! {
                        _ = token.cancelled() => break,
                        _ = interval_timer.tick() => {
                            batch_buffer.clear();

                            // Capture collection time at the end of the interval
                            let collection_time = std::time::SystemTime::now();

                            // Sample all charts in batch to the shared buffer
                            for sampler in &mut batch {
                                sampler.sample_to_buffer(&mut batch_buffer, collection_time).await;
                            }

                            // Single writer lock acquisition for entire batch
                            if !batch_buffer.is_empty() {
                                let mut w = writer.lock().await;
                                let _ = w.write_raw(&batch_buffer).await;
                            }
                        }
                    }
                }
            });
        }

        // Wait for all batch tasks to finish
        while let Some(result) = tasks.join_next().await {
            result?;
        }

        Ok(())
    }
}

/// Internal trait for chart samplers
#[async_trait]
trait ChartSampler: Send + Sync {
    /// Sample the chart and write to the provided buffer (without flushing to stdout)
    ///
    /// # Parameters
    /// - `buffer`: Buffer to write the chart data to
    /// - `collection_time`: When the data was collected
    async fn sample_to_buffer(&mut self, buffer: &mut bytes::BytesMut, collection_time: std::time::SystemTime);
    fn interval(&self) -> Duration;
}

/// Sampler for singleton charts
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
    async fn sample_to_buffer(&mut self, buffer: &mut BytesMut, collection_time: std::time::SystemTime) {
        // Sample the current value
        let current = {
            let guard = self.data.read();
            (*guard).clone()
        };

        // Update tracker
        self.tracker.update(current);

        // Emit definition if first time
        if !self.tracker.defined {
            self.tracker.emit_definition(&mut self.writer);
        }

        // Always emit update - Netdata requires regular updates even if values don't change
        self.tracker.emit_update(&mut self.writer, collection_time);

        // Append writer's buffer to the batch buffer and clear writer
        self.writer.append_to(buffer);
    }

    fn interval(&self) -> Duration {
        self.tracker.interval
    }
}
