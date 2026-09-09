use super::*;

pub(crate) trait ProjectedRowSink {
    fn reset_row(&mut self);
    fn observe_group_value(&mut self, field_index: usize, value: &str);
    fn consume_row(
        &mut self,
        timestamp_usec: u64,
        handle: RecordHandle,
        metrics: QueryFlowMetrics,
    ) -> Result<()>;
}

pub(crate) struct ProjectedGroupingSink<'a> {
    grouped_aggregates: &'a mut ProjectedGroupAccumulator,
    group_by: &'a [String],
    row_group_field_ids: &'a mut [Option<u32>],
    row_missing_values: &'a mut [Option<String>],
    max_groups: usize,
}

impl<'a> ProjectedGroupingSink<'a> {
    pub(crate) fn new(
        grouped_aggregates: &'a mut ProjectedGroupAccumulator,
        group_by: &'a [String],
        row_group_field_ids: &'a mut [Option<u32>],
        row_missing_values: &'a mut [Option<String>],
        max_groups: usize,
    ) -> Self {
        Self {
            grouped_aggregates,
            group_by,
            row_group_field_ids,
            row_missing_values,
            max_groups,
        }
    }
}

impl ProjectedRowSink for ProjectedGroupingSink<'_> {
    fn reset_row(&mut self) {
        self.row_group_field_ids.fill(None);
        for value in self.row_missing_values.iter_mut() {
            let _ = value.take();
        }
    }

    fn observe_group_value(&mut self, field_index: usize, value: &str) {
        match self.grouped_aggregates.find_field_value(field_index, value) {
            Some(field_id) => self.row_group_field_ids[field_index] = Some(field_id),
            None => {
                // Keep an observed value distinct from a missing field after
                // the group cap is full so the row reaches overflow.
                self.row_missing_values[field_index] = Some(value.to_string());
            }
        }
    }

    fn consume_row(
        &mut self,
        timestamp_usec: u64,
        handle: RecordHandle,
        metrics: QueryFlowMetrics,
    ) -> Result<()> {
        self.grouped_aggregates.accumulate_projected(
            self.group_by,
            timestamp_usec,
            handle,
            metrics,
            self.row_group_field_ids,
            self.row_missing_values,
            self.max_groups,
        )
    }
}

pub(crate) struct ProjectedTimeseriesSink<'a> {
    fields: &'a ProjectedFieldTable,
    retained_group_keys: &'a FastHashMap<Vec<u32>, usize>,
    top_group_keys: &'a FastHashMap<Vec<u32>, usize>,
    overflow_dimension: Option<usize>,
    sort_by: SortBy,
    layout: TimeseriesLayout,
    series_buckets: &'a mut [Vec<u64>],
    row_group_field_ids: Vec<u32>,
    row_values_known: bool,
}

impl<'a> ProjectedTimeseriesSink<'a> {
    #[allow(clippy::too_many_arguments)]
    pub(crate) fn new(
        fields: &'a ProjectedFieldTable,
        retained_group_keys: &'a FastHashMap<Vec<u32>, usize>,
        top_group_keys: &'a FastHashMap<Vec<u32>, usize>,
        overflow_dimension: Option<usize>,
        sort_by: SortBy,
        layout: TimeseriesLayout,
        series_buckets: &'a mut [Vec<u64>],
        group_by_len: usize,
    ) -> Self {
        Self {
            fields,
            retained_group_keys,
            top_group_keys,
            overflow_dimension,
            sort_by,
            layout,
            series_buckets,
            row_group_field_ids: vec![0; group_by_len],
            row_values_known: true,
        }
    }
}

impl ProjectedRowSink for ProjectedTimeseriesSink<'_> {
    fn reset_row(&mut self) {
        self.row_group_field_ids.fill(0);
        self.row_values_known = true;
    }

    fn observe_group_value(&mut self, field_index: usize, value: &str) {
        match self.fields.find_field_value(field_index, value) {
            Some(field_id) => self.row_group_field_ids[field_index] = field_id,
            None => self.row_values_known = false,
        }
    }

    fn consume_row(
        &mut self,
        timestamp_usec: u64,
        _handle: RecordHandle,
        metrics: QueryFlowMetrics,
    ) -> Result<()> {
        let dimension_index = if self.row_values_known {
            self.top_group_keys
                .get(self.row_group_field_ids.as_slice())
                .copied()
                .or_else(|| {
                    if self
                        .retained_group_keys
                        .contains_key(self.row_group_field_ids.as_slice())
                    {
                        None
                    } else {
                        self.overflow_dimension
                    }
                })
        } else {
            self.overflow_dimension
        };

        if let Some(dimension_index) = dimension_index {
            accumulate_series_bucket(
                self.series_buckets,
                timestamp_usec,
                self.layout.after,
                self.layout.before,
                self.layout.bucket_seconds,
                dimension_index,
                self.sort_by.metric(metrics),
            );
        }
        Ok(())
    }
}
