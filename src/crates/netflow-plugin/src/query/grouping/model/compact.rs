use super::*;

#[derive(Debug, Clone)]
pub(crate) struct CompactAggregatedFlow {
    pub(crate) flow_id: Option<IndexedFlowId>,
    pub(crate) group_field_ids: Option<Vec<u32>>,
    pub(crate) first_ts: u64,
    pub(crate) last_ts: u64,
    pub(crate) metrics: QueryFlowMetrics,
    pub(crate) bucket_label: Option<&'static str>,
    pub(crate) folded_labels: Option<FoldedGroupedLabels>,
}

impl CompactAggregatedFlow {
    pub(crate) fn new(
        record: &QueryFlowRecord,
        _handle: RecordHandle,
        metrics: QueryFlowMetrics,
        flow_id: IndexedFlowId,
    ) -> Self {
        let mut entry = Self {
            flow_id: Some(flow_id),
            group_field_ids: None,
            first_ts: 0,
            last_ts: 0,
            metrics: QueryFlowMetrics::default(),
            bucket_label: None,
            folded_labels: None,
        };
        entry.update_projected(record.timestamp_usec, metrics);
        entry
    }

    pub(crate) fn new_overflow() -> Self {
        Self::new_synthetic_bucket(OVERFLOW_BUCKET_LABEL)
    }

    pub(crate) fn new_other() -> Self {
        Self::new_synthetic_bucket(OTHER_BUCKET_LABEL)
    }

    pub(crate) fn new_synthetic_bucket(bucket_label: &'static str) -> Self {
        Self {
            flow_id: None,
            group_field_ids: None,
            first_ts: 0,
            last_ts: 0,
            metrics: QueryFlowMetrics::default(),
            bucket_label: Some(bucket_label),
            folded_labels: Some(FoldedGroupedLabels::default()),
        }
    }

    pub(crate) fn new_projected(
        _handle: RecordHandle,
        group_field_ids: Vec<u32>,
        timestamp_usec: u64,
        metrics: QueryFlowMetrics,
    ) -> Self {
        let mut entry = Self {
            flow_id: None,
            group_field_ids: Some(group_field_ids),
            first_ts: 0,
            last_ts: 0,
            metrics: QueryFlowMetrics::default(),
            bucket_label: None,
            folded_labels: None,
        };
        entry.update_projected(timestamp_usec, metrics);
        entry
    }

    pub(crate) fn update(&mut self, record: &QueryFlowRecord, metrics: QueryFlowMetrics) {
        self.update_projected(record.timestamp_usec, metrics);
    }

    pub(crate) fn update_projected(&mut self, timestamp_usec: u64, metrics: QueryFlowMetrics) {
        if self.first_ts == 0 || timestamp_usec < self.first_ts {
            self.first_ts = timestamp_usec;
        }
        if timestamp_usec > self.last_ts {
            self.last_ts = timestamp_usec;
        }
        self.metrics.add(metrics);
    }
}

#[derive(Debug, Default)]
pub(crate) struct CompactGroupOverflow {
    pub(crate) aggregate: Option<CompactAggregatedFlow>,
    pub(crate) dropped_records: u64,
}

pub(crate) struct CompactGroupAccumulator {
    pub(crate) index: FlowIndex,
    pub(crate) rows: Vec<CompactAggregatedFlow>,
    pub(crate) scratch_field_ids: Vec<u32>,
    pub(crate) overflow: CompactGroupOverflow,
}

impl CompactGroupAccumulator {
    pub(crate) fn new(group_by: &[String]) -> Result<Self> {
        let schema = group_by
            .iter()
            .map(|field| IndexFieldSpec::new(field.clone(), IndexFieldKind::Text));
        Ok(Self {
            index: FlowIndex::new(schema)
                .context("failed to build compact flow index for grouped query")?,
            rows: Vec::new(),
            scratch_field_ids: Vec::with_capacity(group_by.len()),
            overflow: CompactGroupOverflow::default(),
        })
    }

    pub(crate) fn grouped_total(&self) -> usize {
        self.rows.len()
    }
}

pub(crate) struct CompactBuildResult {
    pub(crate) flows: Vec<Value>,
    pub(crate) metrics: QueryFlowMetrics,
    pub(crate) grouped_total: usize,
    pub(crate) truncated: bool,
    pub(crate) other_count: usize,
    pub(crate) overflow_records: u64,
}

pub(crate) struct RankedCompactAggregates {
    pub(crate) rows: Vec<CompactAggregatedFlow>,
    pub(crate) other: Option<CompactAggregatedFlow>,
    pub(crate) truncated: bool,
    pub(crate) other_count: usize,
}
