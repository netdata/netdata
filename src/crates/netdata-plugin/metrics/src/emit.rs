//! The only writer of chart-protocol text: chart instance definitions,
//! updates and obsolete marks, packed into chunks.

use crate::chart::ChartTemplate;
use crate::metric_set::{Labels, MetricSetInner};
use netdata_plugin_protocol::{
    ChartLine, ChartOptions, DimensionAlgorithm, DimensionLine, DimensionOptions, LabelSource,
    write_begin, write_chart, write_clabel, write_clabel_commit, write_dimension, write_end_timed,
    write_set_empty, write_set_float,
};

impl MetricSetInner {
    /// The definition of each of an entity's chart instances, whose ids are
    /// `chart_ids`: the CHART line, its labels and dimensions.
    pub(crate) fn write_definitions(
        &self,
        chart_ids: &[String],
        labels: &Labels,
        out: &mut Chunker,
    ) {
        for (template, id) in self.charts.templates.iter().zip(chart_ids) {
            let group = self.schema.column_group(template.group);
            out.block(|block| {
                self.write_chart_line(block, template, id, false);
                if !labels.is_empty() {
                    for (key, value) in labels.iter() {
                        write_clabel(block, key, value, LabelSource::Auto);
                    }
                    write_clabel_commit(block);
                }
                for dimension_id in &group.names {
                    write_dimension(
                        block,
                        &DimensionLine {
                            id: dimension_id,
                            name: dimension_id,
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
            });
        }
    }

    /// One window's update of each of an entity's chart instances: each
    /// dimension's value from `row`, or a gap, timed at the window's end.
    pub(crate) fn write_updates(
        &self,
        chart_ids: &[String],
        row: &[f64],
        window: u64,
        out: &mut Chunker,
    ) {
        let interval_secs = self.interval_secs;
        for (template, id) in self.charts.templates.iter().zip(chart_ids) {
            let group = self.schema.column_group(template.group);
            out.block(|block| {
                write_begin(block, id, interval_secs * 1_000_000);
                for (i, dimension_id) in group.names.iter().enumerate() {
                    match group.value(row, i, interval_secs) {
                        Some(value) => write_set_float(block, dimension_id, value),
                        None => write_set_empty(block, dimension_id),
                    }
                }
                write_end_timed(block, (window + 1) * interval_secs);
            });
        }
    }

    /// The CHART line alone, marked obsolete, marks a chart instance
    /// obsolete: one per chart instance of the entity.
    pub(crate) fn write_obsolete(&self, chart_ids: &[String], out: &mut Chunker) {
        for (template, id) in self.charts.templates.iter().zip(chart_ids) {
            out.block(|block| self.write_chart_line(block, template, id, true));
        }
    }

    /// `store_first` aligns the agent's clock for the chart to whole seconds
    /// and keeps its first point, so every timed END lands exactly on its
    /// window.
    fn write_chart_line(
        &self,
        block: &mut Vec<u8>,
        template: &ChartTemplate,
        id: &str,
        obsolete: bool,
    ) {
        let descriptor = &template.descriptor;
        write_chart(
            block,
            &ChartLine {
                id,
                name: "",
                title: &descriptor.title,
                units: &descriptor.units,
                family: &descriptor.family,
                context: &descriptor.context,
                chart_type: descriptor.chart_type,
                priority: descriptor.priority,
                update_every: self.interval_secs,
                options: ChartOptions {
                    obsolete,
                    store_first: true,
                    hidden: false,
                },
            },
        );
    }
}

/// Packs blocks (one definition, update or obsolete mark each) into chunks
/// under a byte budget.
pub(crate) struct Chunker {
    max_chunk: usize,
    chunks: Vec<Vec<u8>>,
    current: Vec<u8>,
    block: Vec<u8>,
}

impl Chunker {
    pub(crate) fn new(max_chunk: usize) -> Self {
        Self {
            max_chunk,
            chunks: Vec::new(),
            current: Vec::new(),
            block: Vec::new(),
        }
    }

    /// Add one block, written by `write`; a chunk never splits a block.
    pub(crate) fn block(&mut self, write: impl FnOnce(&mut Vec<u8>)) {
        self.block.clear();
        write(&mut self.block);
        if !self.current.is_empty() && self.current.len() + self.block.len() > self.max_chunk {
            self.chunks.push(std::mem::take(&mut self.current));
        }
        self.current.extend_from_slice(&self.block);
    }

    pub(crate) fn finish(mut self) -> Vec<Vec<u8>> {
        if !self.current.is_empty() {
            self.chunks.push(self.current);
        }
        self.chunks
    }
}
