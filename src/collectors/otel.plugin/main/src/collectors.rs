use crate::samples_chart::{SamplePoint, SamplesChart, SamplesChartCollector};
use otel_utils::{DataPointInfo, OtelDataPoint, OtelFormat, ScalarPointInfo, SummaryPointInfo};

use std::fmt::Write;

fn emit_labels<T: OtelDataPoint>(buffer: &mut String, dpi: &DataPointInfo<'_, T>) {
    if let Some(resource) = &dpi.resource_metrics.resource {
        for kv in &resource.attributes {
            write!(buffer, "CLABEL 'resource.{}' '", kv.key).unwrap();
            if let Some(value) = &kv.value {
                value.otel_format(buffer).unwrap();
            }
            writeln!(buffer, "' 1").unwrap();
        }
    }

    if let Some(scope) = &dpi.scope_metrics.scope {
        writeln!(buffer, "CLABEL 'scope.name' '{}' 1", scope.name).unwrap();
        writeln!(buffer, "CLABEL 'scope.version' '{}' 1", scope.version).unwrap();

        for kv in &scope.attributes {
            write!(buffer, "CLABEL 'scope.{}' '", kv.key).unwrap();
            if let Some(value) = &kv.value {
                value.otel_format(buffer).unwrap();
            }
            writeln!(buffer, "' 1").unwrap();
        }
    }

    writeln!(buffer, "CLABEL_COMMIT").unwrap();
}

#[derive(Debug, Default)]
pub struct ScalarChartCollector<'a> {
    collection_time: u64,
    buffer: String,

    chart_name_buffer: String,
    dimension_name_buffer: String,

    scalar_points: Option<&'a [ScalarPointInfo<'a>]>,
}

impl<'a> ScalarChartCollector<'a> {
    pub fn ingest(&mut self, scalar_points: &'a [ScalarPointInfo<'_>]) {
        self.scalar_points = Some(scalar_points);
    }
}

impl SamplesChartCollector for ScalarChartCollector<'_> {
    fn chart_name(&mut self) -> &str {
        let scalar_point = &self.scalar_points.unwrap()[0];
        self.chart_name_buffer.clear();
        scalar_point.instance(&mut self.chart_name_buffer).unwrap();
        &self.chart_name_buffer
    }

    fn emit_sample_points(&mut self, chart: &mut SamplesChart) {
        for scalar_point in self.scalar_points.unwrap() {
            self.dimension_name_buffer.clear();
            scalar_point.name(&mut self.dimension_name_buffer).unwrap();

            let sample_point = SamplePoint {
                unix_time: scalar_point.unix_time(),
                value: scalar_point.value(),
            };

            chart.ingest(&self.dimension_name_buffer, sample_point);
        }

        chart.process(self, 3);
    }

    fn emit_chart_definition(&mut self, update_every: u64) {
        let name = &self.chart_name_buffer;
        let title = &self.scalar_points.unwrap()[0].metric.description;
        let unit = &self.scalar_points.unwrap()[0].metric.unit;
        let family = &self.scalar_points.unwrap()[0].metric.name;
        let context = family;
        let chart_type = "line";
        let update_every = std::time::Duration::from_nanos(update_every).as_secs();

        writeln!(&mut self.buffer, "CHART 'otel.{name}' 'otel.{name}' '{title}' '{unit}' 'otel.{family}' 'otel.{context}' '{chart_type}' 1000 {update_every}").unwrap();

        emit_labels(&mut self.buffer, &self.scalar_points.unwrap()[0]);

        for scalar_point in self.scalar_points.unwrap() {
            self.dimension_name_buffer.clear();
            scalar_point.name(&mut self.dimension_name_buffer).unwrap();

            let id = &self.dimension_name_buffer;
            let name = id;
            let algorithm = if scalar_point.is_monotonic() {
                "incremental"
            } else {
                "absolute"
            };

            writeln!(
                &mut self.buffer,
                "DIMENSION '{id}' '{name}' '{algorithm}' 1 1000"
            )
            .unwrap();
        }
    }

    fn emit_begin(&mut self, collection_time: u64) {
        self.collection_time = collection_time;

        let name = &self.chart_name_buffer;
        writeln!(&mut self.buffer, "BEGIN 'otel.{name}'").unwrap();
    }

    fn emit_set(&mut self, dimension_name: &str, sample_point: &SamplePoint) {
        writeln!(
            &mut self.buffer,
            "SET '{}' {}",
            dimension_name,
            (sample_point.value * 1000.0) as u64
        )
        .unwrap();
    }

    fn emit_end(&mut self, collection_time: u64) {
        let collection_time = std::time::Duration::from_nanos(collection_time).as_secs();
        writeln!(&mut self.buffer, "END {collection_time}").unwrap();

        print!("{}", self.buffer);
        self.buffer.clear();
    }
}

#[derive(Debug, Default)]
pub struct SummaryChartCollector<'a> {
    collection_time: u64,
    buffer: String,

    chart_name_buffer: String,
    dimension_name_buffer: String,

    summary_points: Option<&'a [SummaryPointInfo<'a>]>,
}

impl<'a> SummaryChartCollector<'a> {
    pub fn ingest(&mut self, summary_points: &'a [SummaryPointInfo<'_>]) {
        self.summary_points = Some(summary_points);
    }
}

impl SamplesChartCollector for SummaryChartCollector<'_> {
    fn chart_name(&mut self) -> &str {
        let summary_point = &self.summary_points.unwrap()[0];

        self.chart_name_buffer.clear();
        summary_point.instance(&mut self.chart_name_buffer).unwrap();
        &self.chart_name_buffer
    }

    fn emit_sample_points(&mut self, chart: &mut SamplesChart) {
        for summary_point in self.summary_points.unwrap() {
            self.dimension_name_buffer.clear();
            summary_point.name(&mut self.dimension_name_buffer).unwrap();

            let n = self.dimension_name_buffer.len();
            for bucket in &summary_point.point.quantile_values {
                write!(&mut self.dimension_name_buffer, "_{}", bucket.quantile).unwrap();

                let sample_point = SamplePoint {
                    unix_time: summary_point.point.time_unix_nano,
                    value: bucket.value,
                };
                chart.ingest(&self.dimension_name_buffer, sample_point);

                self.dimension_name_buffer.truncate(n);
            }
        }

        chart.process(self, 3);
    }

    fn emit_chart_definition(&mut self, update_every: u64) {
        let name = &self.chart_name_buffer;
        let title = &self.summary_points.unwrap()[0].metric.description;
        let unit = &self.summary_points.unwrap()[0].metric.unit;
        let family = &self.summary_points.unwrap()[0].metric.name;
        let context = family;
        let chart_type = "heatmap";
        let update_every = std::time::Duration::from_nanos(update_every).as_secs();

        writeln!(&mut self.buffer, "CHART 'otel.{name}' 'otel.{name}' '{title}' '{unit}' 'otel.{family}' 'otel.{context}' '{chart_type}' 1000 {update_every}").unwrap();

        emit_labels(&mut self.buffer, &self.summary_points.unwrap()[0]);

        for summary_point in self.summary_points.unwrap() {
            self.dimension_name_buffer.clear();
            summary_point.name(&mut self.dimension_name_buffer).unwrap();

            let n = self.dimension_name_buffer.len();
            for bucket in &summary_point.point.quantile_values {
                write!(&mut self.dimension_name_buffer, "_{}", bucket.quantile).unwrap();

                let id = &self.dimension_name_buffer;
                let name = id;
                let algorithm = "absolute";

                writeln!(
                    &mut self.buffer,
                    "DIMENSION '{id}' '{name}' '{algorithm}' 1 1000"
                )
                .unwrap();

                self.dimension_name_buffer.truncate(n);
            }
        }
    }

    fn emit_begin(&mut self, collection_time: u64) {
        self.collection_time = collection_time;

        let name = &self.chart_name_buffer;
        writeln!(&mut self.buffer, "BEGIN 'otel.{name}'").unwrap();
    }

    fn emit_set(&mut self, dimension_name: &str, sample_point: &SamplePoint) {
        writeln!(
            &mut self.buffer,
            "SET '{}' {}",
            dimension_name,
            (sample_point.value * 1000.0) as u64
        )
        .unwrap();
    }

    fn emit_end(&mut self, collection_time: u64) {
        let collection_time = std::time::Duration::from_nanos(collection_time).as_secs();
        writeln!(&mut self.buffer, "END {collection_time}").unwrap();

        print!("{}", self.buffer);
        self.buffer.clear();
    }
}
