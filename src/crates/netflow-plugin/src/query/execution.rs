use super::*;
use rt::ProgressState;
use std::sync::atomic::{AtomicUsize, Ordering};
use tokio_util::sync::CancellationToken;

const QUERY_CHECKPOINT_INTERVAL: u64 = 1024;

#[derive(Clone)]
pub(crate) struct QueryExecutionContext {
    pub(crate) progress: ProgressState,
    pub(crate) cancellation: CancellationToken,
}

impl QueryExecutionContext {
    pub(crate) fn new(progress: ProgressState, cancellation: CancellationToken) -> Self {
        Self {
            progress,
            cancellation,
        }
    }
}

pub(crate) struct QueryExecutionPlan {
    cancellation: CancellationToken,
    progress: ProgressState,
    span_after_seconds: Vec<usize>,
    span_prefix_seconds: Vec<usize>,
    pass_span_seconds: usize,
    total_seconds: usize,
    last_done_seconds: AtomicUsize,
}

impl QueryExecutionPlan {
    pub(crate) fn for_flows(setup: &QuerySetup, ctx: QueryExecutionContext) -> Self {
        Self::new(setup, 1, ctx)
    }

    pub(crate) fn for_timeseries(setup: &QuerySetup, ctx: QueryExecutionContext) -> Self {
        Self::new(setup, 2, ctx)
    }

    fn new(setup: &QuerySetup, pass_count: usize, ctx: QueryExecutionContext) -> Self {
        let mut span_after_seconds = Vec::with_capacity(setup.spans.len());
        let mut span_prefix_seconds = Vec::with_capacity(setup.spans.len());
        let mut pass_span_seconds = 0usize;
        for span in &setup.spans {
            span_after_seconds.push(span.span.after as usize);
            span_prefix_seconds.push(pass_span_seconds);
            let span_seconds = (span.span.before.saturating_sub(span.span.after)) as usize;
            pass_span_seconds = pass_span_seconds.saturating_add(span_seconds);
        }

        let total_seconds = pass_span_seconds.saturating_mul(pass_count).max(1);
        ctx.progress.update(0, total_seconds);

        Self {
            cancellation: ctx.cancellation,
            progress: ctx.progress,
            span_after_seconds,
            span_prefix_seconds,
            pass_span_seconds,
            total_seconds,
            last_done_seconds: AtomicUsize::new(0),
        }
    }

    pub(crate) fn checkpoint(
        &self,
        pass_index: usize,
        span_index: usize,
        streamed_entries: u64,
        timestamp_usec: u64,
    ) -> Result<()> {
        if streamed_entries.is_multiple_of(QUERY_CHECKPOINT_INTERVAL) {
            self.check_cancelled()?;
            self.report_timestamp(pass_index, span_index, timestamp_usec);
        }
        Ok(())
    }

    pub(crate) fn start_span(&self, pass_index: usize, span_index: usize) -> Result<()> {
        self.check_cancelled()?;
        self.report_done(self.span_start_done(pass_index, span_index));
        Ok(())
    }

    pub(crate) fn finish_span(&self, pass_index: usize, span_index: usize) -> Result<()> {
        self.check_cancelled()?;
        self.report_done(self.span_end_done(pass_index, span_index));
        Ok(())
    }

    pub(crate) fn finish(&self) {
        self.report_done(self.total_seconds);
    }

    fn check_cancelled(&self) -> Result<()> {
        if self.cancellation.is_cancelled() {
            anyhow::bail!("netflow flow query cancelled");
        }
        Ok(())
    }

    fn report_timestamp(&self, pass_index: usize, span_index: usize, timestamp_usec: u64) {
        if span_index >= self.span_prefix_seconds.len() {
            return;
        }

        let start_done = self.span_start_done(pass_index, span_index);
        let span_seconds = self.span_duration_seconds(span_index);

        // Convert the current entry timestamp to whole seconds within the current span.
        // We clamp aggressively to keep progress monotonic and bounded.
        let span_after = self.current_span_after_seconds(span_index);
        let timestamp_seconds = (timestamp_usec / 1_000_000) as usize;
        let within_span = timestamp_seconds
            .saturating_sub(span_after)
            .min(span_seconds);

        self.report_done(start_done.saturating_add(within_span));
    }

    fn report_done(&self, done: usize) {
        let next = done
            .min(self.total_seconds)
            .max(self.last_done_seconds.load(Ordering::Relaxed));
        self.last_done_seconds.store(next, Ordering::Relaxed);
        self.progress.update(next, self.total_seconds);
    }

    fn span_start_done(&self, pass_index: usize, span_index: usize) -> usize {
        self.pass_offset_seconds(pass_index).saturating_add(
            self.span_prefix_seconds
                .get(span_index)
                .copied()
                .unwrap_or(0),
        )
    }

    fn span_end_done(&self, pass_index: usize, span_index: usize) -> usize {
        self.span_start_done(pass_index, span_index)
            .saturating_add(self.span_duration_seconds(span_index))
    }

    fn span_duration_seconds(&self, span_index: usize) -> usize {
        let start = self
            .span_prefix_seconds
            .get(span_index)
            .copied()
            .unwrap_or(0);
        let end = self
            .span_prefix_seconds
            .get(span_index + 1)
            .copied()
            .unwrap_or(self.pass_span_seconds);
        end.saturating_sub(start)
    }

    fn pass_offset_seconds(&self, pass_index: usize) -> usize {
        self.pass_span_seconds.saturating_mul(pass_index)
    }

    fn current_span_after_seconds(&self, span_index: usize) -> usize {
        self.span_after_seconds
            .get(span_index)
            .copied()
            .unwrap_or_default()
    }
}
