use super::*;

#[derive(Debug, Clone, Default)]
pub(crate) struct FoldedGroupedLabels {
    pub(crate) values: BTreeMap<String, BTreeSet<String>>,
}

impl FoldedGroupedLabels {
    pub(crate) fn merge_labels(&mut self, labels: &BTreeMap<String, String>) {
        for (field, value) in labels {
            if field == "_bucket" {
                continue;
            }
            self.values
                .entry(field.clone())
                .or_default()
                .insert(value.clone());
        }
    }

    pub(crate) fn merge_folded(&mut self, other: &Self) {
        for (field, values) in &other.values {
            self.values
                .entry(field.clone())
                .or_default()
                .extend(values.iter().cloned());
        }
    }

    pub(crate) fn render_into(&self, labels: &mut BTreeMap<String, String>) {
        for (field, values) in &self.values {
            if values.is_empty() {
                continue;
            }

            let rendered = if values.len() == 1 {
                values.iter().next().cloned().unwrap_or_default()
            } else {
                format!("Other ({})", values.len())
            };
            labels.insert(field.clone(), rendered);
        }
    }
}

#[cfg(test)]
#[allow(dead_code)]
pub(crate) struct BuildResult {
    pub(crate) flows: Vec<Value>,
    pub(crate) metrics: QueryFlowMetrics,
    pub(crate) returned: usize,
    pub(crate) grouped_total: usize,
    pub(crate) truncated: bool,
    pub(crate) other_count: usize,
}

pub(crate) struct RankedAggregates {
    pub(crate) rows: Vec<AggregatedFlow>,
    #[cfg(test)]
    pub(crate) other: Option<AggregatedFlow>,
    pub(crate) grouped_total: usize,
    pub(crate) truncated: bool,
    pub(crate) other_count: usize,
}

#[derive(Debug, Default)]
pub(crate) struct AggregatedFlow {
    pub(crate) labels: BTreeMap<String, String>,
    pub(crate) first_ts: u64,
    pub(crate) last_ts: u64,
    pub(crate) metrics: QueryFlowMetrics,
    pub(crate) folded_labels: Option<FoldedGroupedLabels>,
}

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub(crate) struct GroupKey(pub(crate) Vec<(String, String)>);

#[derive(Debug, Default)]
pub(crate) struct GroupOverflow {
    pub(crate) aggregate: Option<AggregatedFlow>,
    pub(crate) dropped_records: u64,
}

#[derive(Debug, Default)]
#[cfg(test)]
pub(crate) struct FacetFieldAccumulator {
    pub(crate) values: BTreeMap<String, QueryFlowMetrics>,
    pub(crate) overflow_metrics: QueryFlowMetrics,
    pub(crate) overflow_records: u64,
}
