//! The only writer of chart-protocol text: chart instance definitions,
//! updates and obsolete marks, packed into chunks.

use crate::chart::ChartTemplate;
use crate::metric_set::Labels;
use netdata_plugin_protocol::{
    ChartLine, ChartOptions, DimensionAlgorithm, DimensionLine, DimensionOptions, LabelSource,
    write_begin, write_chart, write_clabel, write_clabel_commit, write_dimension, write_end_timed,
    write_set_empty, write_set_float,
};

/// A chart instance's definition: the CHART line, its labels and dimensions.
pub(crate) fn write_definition(
    block: &mut Vec<u8>,
    template: &ChartTemplate,
    id: &str,
    labels: &Labels,
    interval_secs: u64,
) {
    write_chart_line(block, template, id, interval_secs, false);
    if !labels.is_empty() {
        for (key, value) in labels.iter() {
            write_clabel(block, key, value, LabelSource::Auto);
        }
        write_clabel_commit(block);
    }
    for dimension_id in &template.dimension_ids {
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
}

/// One window's update of a chart instance: each dimension's value from
/// `row`, or a gap, timed at the window's end.
pub(crate) fn write_update(
    block: &mut Vec<u8>,
    template: &ChartTemplate,
    id: &str,
    row: &[f64],
    window: u64,
    interval_secs: u64,
) {
    write_begin(block, id, interval_secs * 1_000_000);
    for (i, dimension_id) in template.dimension_ids.iter().enumerate() {
        match template.value(row, i, interval_secs) {
            Some(value) => write_set_float(block, dimension_id, value),
            None => write_set_empty(block, dimension_id),
        }
    }
    write_end_timed(block, (window + 1) * interval_secs);
}

/// The CHART line alone, marked obsolete, marks a chart instance obsolete.
pub(crate) fn write_obsolete(
    block: &mut Vec<u8>,
    template: &ChartTemplate,
    id: &str,
    interval_secs: u64,
) {
    write_chart_line(block, template, id, interval_secs, true);
}

/// `store_first` aligns the agent's clock for the chart to whole seconds and
/// keeps its first point, so every timed END lands exactly on its window.
fn write_chart_line(
    block: &mut Vec<u8>,
    template: &ChartTemplate,
    id: &str,
    interval_secs: u64,
    obsolete: bool,
) {
    write_chart(
        block,
        &ChartLine {
            id,
            name: "",
            title: &template.title,
            units: template.units,
            family: template.family,
            context: &template.context,
            chart_type: template.chart_type,
            priority: template.priority,
            update_every: interval_secs,
            options: ChartOptions {
                obsolete,
                store_first: true,
                hidden: false,
            },
        },
    );
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
