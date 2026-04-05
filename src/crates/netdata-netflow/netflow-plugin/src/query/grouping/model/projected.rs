use super::*;

pub(crate) struct ProjectedFieldTable {
    pub(crate) value_ids: Vec<FastHashMap<String, u32>>,
    pub(crate) values: Vec<Vec<String>>,
}

impl ProjectedFieldTable {
    pub(crate) fn new(group_by: &[String]) -> Self {
        let mut value_ids = Vec::with_capacity(group_by.len());
        let mut values = Vec::with_capacity(group_by.len());
        for _ in group_by {
            value_ids.push(FastHashMap::from([(String::new(), 0)]));
            values.push(vec![String::new()]);
        }
        Self { value_ids, values }
    }

    pub(crate) fn find_field_value(&self, field_index: usize, value: &str) -> Option<u32> {
        self.value_ids
            .get(field_index)
            .and_then(|values| values.get(value).copied())
    }

    pub(crate) fn get_or_insert_field_value(&mut self, field_index: usize, value: &str) -> u32 {
        if let Some(field_id) = self.find_field_value(field_index, value) {
            return field_id;
        }

        let field_values = &mut self.values[field_index];
        let field_id = field_values.len() as u32;
        let owned = value.to_string();
        field_values.push(owned.clone());
        self.value_ids[field_index].insert(owned, field_id);
        field_id
    }

    pub(crate) fn field_value(&self, field_index: usize, field_id: u32) -> Option<&str> {
        self.values
            .get(field_index)
            .and_then(|values| values.get(field_id as usize))
            .map(String::as_str)
    }
}

pub(crate) struct ProjectedGroupAccumulator {
    pub(crate) fields: ProjectedFieldTable,
    pub(crate) rows: Vec<CompactAggregatedFlow>,
    pub(crate) row_indexes: FastHashMap<Vec<u32>, usize>,
    pub(crate) scratch_field_ids: Vec<u32>,
    pub(crate) overflow: CompactGroupOverflow,
}

impl ProjectedGroupAccumulator {
    pub(crate) fn new(group_by: &[String]) -> Self {
        Self {
            fields: ProjectedFieldTable::new(group_by),
            rows: Vec::new(),
            row_indexes: FastHashMap::default(),
            scratch_field_ids: Vec::with_capacity(group_by.len()),
            overflow: CompactGroupOverflow::default(),
        }
    }

    pub(crate) fn grouped_total(&self) -> usize {
        self.rows.len()
    }

    pub(crate) fn find_field_value(&self, field_index: usize, value: &str) -> Option<u32> {
        self.fields.find_field_value(field_index, value)
    }

    pub(crate) fn accumulate_projected(
        &mut self,
        group_by: &[String],
        timestamp_usec: u64,
        handle: RecordHandle,
        metrics: QueryFlowMetrics,
        row_group_field_ids: &mut [Option<u32>],
        row_missing_values: &mut [Option<String>],
        max_groups: usize,
    ) -> Result<()> {
        anyhow::ensure!(
            row_group_field_ids.len() == row_missing_values.len(),
            "projected grouped row buffers are misaligned"
        );

        self.scratch_field_ids.clear();
        let mut needs_field_inserts = false;
        for field_index in 0..row_group_field_ids.len() {
            if let Some(field_id) = row_group_field_ids[field_index] {
                self.scratch_field_ids.push(field_id);
                continue;
            }

            if row_missing_values[field_index].is_some() {
                needs_field_inserts = true;
                break;
            }

            self.scratch_field_ids.push(0);
        }

        if !needs_field_inserts {
            if let Some(row_index) = self.row_indexes.get(self.scratch_field_ids.as_slice()) {
                if let Some(entry) = self.rows.get_mut(*row_index) {
                    entry.update_projected(timestamp_usec, metrics);
                    return Ok(());
                }
                anyhow::bail!(
                    "projected grouped index returned missing row for row slot {}",
                    row_index
                );
            }

            if self.grouped_total() >= max_groups {
                let entry = self
                    .overflow
                    .aggregate
                    .get_or_insert_with(CompactAggregatedFlow::new_overflow);
                self.overflow.dropped_records = self.overflow.dropped_records.saturating_add(1);
                let labels = projected_group_labels_from_fields(
                    &self.fields,
                    group_by,
                    row_group_field_ids,
                    row_missing_values,
                )?;
                merge_compact_projected_labels(entry, &labels);
                entry.update_projected(timestamp_usec, metrics);
                return Ok(());
            }

            let row_index = self.rows.len();
            self.row_indexes
                .insert(self.scratch_field_ids.clone(), row_index);
            self.rows.push(CompactAggregatedFlow::new_projected(
                handle,
                self.scratch_field_ids.clone(),
                timestamp_usec,
                metrics,
            ));
            return Ok(());
        }

        if self.grouped_total() >= max_groups {
            let entry = self
                .overflow
                .aggregate
                .get_or_insert_with(CompactAggregatedFlow::new_overflow);
            self.overflow.dropped_records = self.overflow.dropped_records.saturating_add(1);
            let labels = projected_group_labels_from_fields(
                &self.fields,
                group_by,
                row_group_field_ids,
                row_missing_values,
            )?;
            merge_compact_projected_labels(entry, &labels);
            entry.update_projected(timestamp_usec, metrics);
            return Ok(());
        }

        self.scratch_field_ids.clear();
        for field_index in 0..row_group_field_ids.len() {
            let field_id = if let Some(field_id) = row_group_field_ids[field_index] {
                field_id
            } else if let Some(value) = row_missing_values[field_index].take() {
                let field_id = self
                    .fields
                    .get_or_insert_field_value(field_index, value.as_str());
                row_group_field_ids[field_index] = Some(field_id);
                field_id
            } else {
                0
            };
            self.scratch_field_ids.push(field_id);
        }

        if let Some(row_index) = self.row_indexes.get(self.scratch_field_ids.as_slice()) {
            if let Some(entry) = self.rows.get_mut(*row_index) {
                entry.update_projected(timestamp_usec, metrics);
                return Ok(());
            }
            anyhow::bail!(
                "projected grouped index returned missing row for row slot {}",
                row_index
            );
        }

        let row_index = self.rows.len();
        self.row_indexes
            .insert(self.scratch_field_ids.clone(), row_index);
        self.rows.push(CompactAggregatedFlow::new_projected(
            handle,
            self.scratch_field_ids.clone(),
            timestamp_usec,
            metrics,
        ));
        Ok(())
    }
}
