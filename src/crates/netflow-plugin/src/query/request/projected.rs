#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub(crate) struct ProjectedPayloadAction {
    pub(crate) group_slot: Option<usize>,
    pub(crate) capture_slot: Option<usize>,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) enum ProjectedMetricField {
    Bytes,
    Packets,
}

#[derive(Clone, Copy, Debug, Default)]
pub(crate) struct ProjectedFieldTargets {
    pub(crate) metric: Option<ProjectedMetricField>,
    pub(crate) action: ProjectedPayloadAction,
}

#[derive(Clone, Debug)]
pub(crate) struct ProjectedFieldSpec {
    pub(crate) prefix: u64,
    pub(crate) mask: u64,
    pub(crate) key: Vec<u8>,
    pub(crate) targets: ProjectedFieldTargets,
}

#[derive(Clone, Debug)]
pub(crate) struct ProjectedFieldMatchPlan {
    pub(crate) first_byte_masks: [u64; 256],
    pub(crate) all_mask: u64,
    pub(crate) all_keys_fit_prefix: bool,
}

impl ProjectedFieldMatchPlan {
    pub(crate) fn new(specs: &[ProjectedFieldSpec]) -> Option<Self> {
        if specs.is_empty() || specs.len() > u64::BITS as usize {
            return None;
        }

        let mut first_byte_masks = [0_u64; 256];
        for (index, spec) in specs.iter().enumerate() {
            let first = *spec.key.first()?;
            first_byte_masks[first as usize] |= 1_u64 << index;
        }

        let all_mask = if specs.len() == u64::BITS as usize {
            u64::MAX
        } else {
            (1_u64 << specs.len()) - 1
        };
        let all_keys_fit_prefix = specs.iter().all(|spec| spec.key.len() <= 8);

        Some(Self {
            first_byte_masks,
            all_mask,
            all_keys_fit_prefix,
        })
    }
}

#[cfg(test)]
pub(crate) struct RawScanBenchResult {
    pub(crate) files_opened: u64,
    pub(crate) rows_read: u64,
    pub(crate) fields_read: u64,
    pub(crate) elapsed_usec: u128,
}

#[cfg(test)]
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) enum RawProjectedBenchStage {
    MatchOnly,
    MatchAndExtract,
    MatchExtractAndParseMetrics,
    GroupAndAccumulate,
}

#[cfg(test)]
pub(crate) struct RawProjectedBenchResult {
    pub(crate) files_opened: u64,
    pub(crate) rows_read: u64,
    pub(crate) fields_read: u64,
    pub(crate) processed_fields: u64,
    pub(crate) compressed_processed_fields: u64,
    pub(crate) matched_entries: u64,
    pub(crate) grouped_rows: u64,
    pub(crate) work_checksum: u64,
    pub(crate) elapsed_usec: u128,
}
