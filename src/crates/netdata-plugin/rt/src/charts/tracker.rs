//! Emission state for one registered chart.
//!
//! [`TrackedChart`] holds what a sample tick needs to emit a chart: the latest
//! and previous sampled values (change detection), the resolved
//! [`ChartMetadata`], the sample interval, and whether the CHART/DIMENSION
//! definition has been emitted yet. The registry (`super::registry`) owns one
//! tracker per chart inside its `SingletonChartSampler` and drives it once per
//! tick: it clones the newest value out of the chart's `ChartHandle`
//! (`super::handle`), calls `update`, emits the definition on the first tick,
//! then BEGIN/SET/END on every tick. Emission is unconditional — change
//! detection is informational only — so each sample interval carries a
//! datapoint. Commands go into a per-sampler [`ChartWriter`] whose buffer the
//! sampler appends to the shared batch buffer; the tracker never touches the
//! outbound stream. Plugins normally reach this through chart registration
//! (`PluginRuntime::register_chart` / `register_instanced_chart`).

use super::chart_trait::{InstancedChart, NetdataChart};
use super::metadata::ChartMetadata;
use super::writer::ChartWriter;
use std::time::{Duration, SystemTime};

/// One chart's emission state, owned by the registry's `SingletonChartSampler`.
///
/// Both stored values start at the constructor's `initial`, so `has_changed()`
/// is false until the first `update()`.
pub struct TrackedChart<T> {
    /// Newest sampled value, written into every update's SET commands.
    current: T,
    /// The value before the last `update()`; compared against `current` by
    /// `has_changed()`.
    previous: T,
    /// Resolved chart definition. For instanced charts this is an
    /// instantiated copy: `new_instanced` substitutes the instance id for
    /// `{instance}` (id, name, title, family, context) at construction, and
    /// later `update()` calls never change it.
    pub(crate) metadata: ChartMetadata,
    /// Sample interval; also emitted as the update-every field (in
    /// microseconds) of every BEGIN command.
    pub(crate) interval: Duration,
    /// Set by the first `emit_definition` call; the registry's sampler reads
    /// it directly to gate its per-tick definition emission.
    pub(crate) defined: bool,
}

impl<T: NetdataChart + Default + PartialEq + Clone> TrackedChart<T> {
    /// Create a tracker whose metadata comes from `T::chart_metadata()` (the
    /// type's schemars `x-chart-*` annotations). Used by
    /// `ChartRegistry::register_chart` for singleton charts.
    pub fn new(initial: T, interval: Duration) -> Self {
        let metadata = T::chart_metadata();
        Self::new_with_metadata(initial, interval, metadata)
    }

    /// Like `new`, but with caller-supplied metadata instead of the type's
    /// schema annotations. Only `new` calls it.
    pub(crate) fn new_with_metadata(
        initial: T,
        interval: Duration,
        metadata: ChartMetadata,
    ) -> Self {
        Self {
            previous: initial.clone(),
            current: initial,
            metadata,
            interval,
            defined: false,
        }
    }

    /// Advance the sample: the current value moves to `previous` and
    /// `new_values` becomes `current`. The registry's sampler calls this once
    /// per tick with the value cloned from the chart's `ChartHandle`.
    pub fn update(&mut self, new_values: T) {
        self.previous = std::mem::replace(&mut self.current, new_values);
    }

    /// Whether the latest sample differs from the previous one.
    ///
    /// Informational only: the registry's sampler ignores this and emits an
    /// update on every tick (see `SingletonChartSampler::sample_to_buffer`),
    /// so each sample interval carries a datapoint.
    pub fn has_changed(&self) -> bool {
        self.current != self.previous
    }

    /// Write the CHART and DIMENSION definition lines to the writer, once per
    /// tracker lifetime; later calls are no-ops.
    ///
    /// Must precede the first [`Self::emit_update`]: an update carries only
    /// BEGIN/SET/END, and the agent disables the plugin when a BEGIN names a
    /// chart it has never seen declared (`pluginsd_begin` in
    /// src/plugins.d/pluginsd_parser.c).
    pub fn emit_definition(&mut self, writer: &mut ChartWriter) {
        if self.defined {
            return;
        }
        self.defined = true;
        writer.write_chart_definition(&self.metadata);
    }

    /// Write one update: BEGIN (carrying the tracker's interval as the
    /// update-every field, in microseconds), the current value's SET lines via
    /// `super::chart_trait::ChartDimensions::write_dimensions`, and END
    /// timestamped with `collection_time` (Unix seconds). The definition is
    /// not emitted here — call [`Self::emit_definition`] first. The registry's
    /// sampler calls this on every tick with the batch's collection time.
    pub fn emit_update(&self, writer: &mut ChartWriter, collection_time: SystemTime)
    where
        T: super::chart_trait::ChartDimensions,
    {
        writer.begin_chart(&self.metadata.id, self.interval);
        self.current.write_dimensions(writer);
        writer.end_chart(collection_time);
    }
}

impl<T: InstancedChart + Default + PartialEq> TrackedChart<T> {
    /// Create a tracker for one instance of an instanced chart: the template
    /// metadata's `{instance}` placeholders are replaced with the initial
    /// value's `instance_id()` (`ChartMetadata::instantiate`), so the tracker
    /// declares a concrete chart. Used by
    /// `ChartRegistry::register_instanced_chart`; the instance is fixed at
    /// construction — later `update()` calls never re-instantiate.
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
        let initial = TestMetrics {
            value1: 10,
            value2: 20,
        };
        let mut tracker = TrackedChart::new(initial.clone(), Duration::from_secs(1));

        assert!(!tracker.has_changed());

        tracker.update(TestMetrics {
            value1: 15,
            value2: 20,
        });
        assert!(tracker.has_changed());

        tracker.update(TestMetrics {
            value1: 15,
            value2: 20,
        });
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
