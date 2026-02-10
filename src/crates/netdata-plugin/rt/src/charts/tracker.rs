//! Tracked chart with change detection and emission.

use super::chart_trait::{InstancedChart, NetdataChart};
use super::metadata::ChartMetadata;
use super::writer::ChartWriter;
use std::time::{Duration, SystemTime};

/// A tracked chart that detects changes and emits Netdata protocol commands.
pub struct TrackedChart<T> {
    current: T,
    previous: T,
    pub(crate) metadata: ChartMetadata,
    pub(crate) interval: Duration,
    pub(crate) defined: bool,
}

impl<T: NetdataChart + Default + PartialEq + Clone> TrackedChart<T> {
    /// Create a new tracked chart with the given initial value and interval
    pub fn new(initial: T, interval: Duration) -> Self {
        let metadata = T::chart_metadata();
        Self::new_with_metadata(initial, interval, metadata)
    }

    /// Create a new tracked chart with explicit metadata (used for instantiated templates)
    pub(crate) fn new_with_metadata(initial: T, interval: Duration, metadata: ChartMetadata) -> Self {
        Self {
            previous: initial.clone(),
            current: initial,
            metadata,
            interval,
            defined: false,
        }
    }

    /// Update the current values
    pub fn update(&mut self, new_values: T) {
        self.previous = std::mem::replace(&mut self.current, new_values);
    }

    /// Check if values changed since last update
    ///
    /// Note: This is provided for informational purposes (e.g., logging, debugging).
    /// The registry always emits updates regardless of changes, as required by Netdata's protocol.
    pub fn has_changed(&self) -> bool {
        self.current != self.previous
    }

    /// Emit the chart definition (CHART + DIMENSION commands) to the writer
    ///
    /// This should be called once before emitting any updates.
    pub fn emit_definition(&mut self, writer: &mut ChartWriter) {
        if self.defined {
            return;
        }
        self.defined = true;
        writer.write_chart_definition(&self.metadata);
    }

    /// Emit a chart update (BEGIN + SET + END commands) to the writer
    ///
    /// This uses the ChartDimensions trait for efficient zero-allocation dimension writing.
    ///
    /// # Parameters
    /// - `writer`: The writer to emit the update to
    /// - `collection_time`: When the data was collected
    pub fn emit_update(&self, writer: &mut ChartWriter, collection_time: SystemTime)
    where
        T: super::chart_trait::ChartDimensions,
    {
        writer.begin_chart(&self.metadata.id, self.interval);
        self.current.write_dimensions(writer);
        writer.end_chart(collection_time);
    }
}

// For instanced charts, we need special handling
impl<T: InstancedChart + Default + PartialEq> TrackedChart<T> {
    /// Create a tracked chart for an instanced chart
    pub fn new_instanced(initial: T, interval: Duration) -> Self {
        let template_metadata = T::chart_metadata();
        let instance_id = initial.instance_id();
        let metadata = template_metadata.instantiate(instance_id);

        Self {
            previous: initial.clone(),
            current: initial,
            metadata,
            interval,
            defined: false,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use schemars::JsonSchema;
    use serde::{Deserialize, Serialize};

    #[derive(JsonSchema, Default, Clone, PartialEq, Serialize, Deserialize)]
    #[schemars(
        extend("x-chart-id" = "test.metrics"),
        extend("x-chart-title" = "Test Metrics"),
    )]
    struct TestMetrics {
        value1: u64,
        value2: u64,
    }

    impl super::super::ChartDimensions for TestMetrics {
        fn write_dimensions(&self, writer: &mut ChartWriter) {
            writer.write_dimension("value1", self.value1 as i64);
            writer.write_dimension("value2", self.value2 as i64);
        }
    }

    #[test]
    fn test_change_detection() {
        let initial = TestMetrics { value1: 10, value2: 20 };
        let mut tracker = TrackedChart::new(initial.clone(), Duration::from_secs(1));

        assert!(!tracker.has_changed());

        tracker.update(TestMetrics { value1: 15, value2: 20 });
        assert!(tracker.has_changed());

        tracker.update(TestMetrics { value1: 15, value2: 20 });
        assert!(!tracker.has_changed());
    }

    #[test]
    fn test_emit_definition() {
        let initial = TestMetrics::default();
        let mut tracker = TrackedChart::new(initial, Duration::from_secs(1));
        let mut writer = ChartWriter::new();

        tracker.emit_definition(&mut writer);
        let def = String::from_utf8_lossy(writer.buffer());
        assert!(def.contains("CHART test.metrics"));
        assert!(def.contains("DIMENSION value1"));
        assert!(def.contains("DIMENSION value2"));

        // Second call should not write anything
        writer.clear();
        tracker.emit_definition(&mut writer);
        assert_eq!(writer.buffer_len(), 0);
    }
}
