use super::super::model::TierFlowRef;
use super::super::rollup::{
    HOUR_BUCKET_USEC, bucket_start_usec, build_rollup_flow_index, materialize_rollup_fields,
    push_rollup_field_ids,
};
#[cfg(test)]
use super::super::rollup::{
    INTERNAL_DIRECTION_PRESENT, INTERNAL_EXPORTER_IP_PRESENT, INTERNAL_NEXT_HOP_PRESENT,
    compact_index_value_to_string, direction_from_u8, rollup_field_value, rollup_presence_field,
};
use crate::flow::{FlowFields, FlowRecord};
use anyhow::{Context, Result, anyhow};
#[cfg(test)]
use netdata_flow_index::FieldValue as IndexFieldValue;
use netdata_flow_index::FlowIndex;
use std::collections::{BTreeMap, BTreeSet};

#[derive(Default)]
pub(crate) struct TierFlowIndexStore {
    generation: u64,
    indexes: BTreeMap<u64, FlowIndex>,
    scratch_field_ids: Vec<u32>,
}

impl TierFlowIndexStore {
    pub(crate) fn generation(&self) -> u64 {
        self.generation
    }

    pub(crate) fn get_or_insert_record_flow(
        &mut self,
        timestamp_usec: u64,
        record: &FlowRecord,
    ) -> Result<TierFlowRef> {
        if timestamp_usec == 0 {
            return Err(anyhow!("tier timestamp cannot be zero"));
        }

        let hour_start_usec = bucket_start_usec(timestamp_usec, HOUR_BUCKET_USEC);
        if !self.indexes.contains_key(&hour_start_usec) {
            self.indexes.insert(
                hour_start_usec,
                build_rollup_flow_index().expect("rollup schema should be valid"),
            );
            self.generation = self.generation.saturating_add(1);
        }

        let index = self
            .indexes
            .get_mut(&hour_start_usec)
            .expect("hour index should exist after insertion");

        self.scratch_field_ids.clear();
        push_rollup_field_ids(index, record, &mut self.scratch_field_ids)
            .context("failed to intern tier flow dimensions")?;

        let flow_id = if let Some(existing) = index
            .find_flow_by_field_ids(&self.scratch_field_ids)
            .context("failed to find tier flow dimensions")?
        {
            existing
        } else {
            index
                .insert_flow_by_field_ids(&self.scratch_field_ids)
                .context("failed to insert tier flow dimensions")?
        };

        Ok(TierFlowRef {
            hour_start_usec,
            flow_id,
        })
    }

    pub(crate) fn materialize_fields(&self, flow_ref: TierFlowRef) -> Option<FlowFields> {
        let index = self.indexes.get(&flow_ref.hour_start_usec)?;
        materialize_rollup_fields(index, flow_ref.flow_id)
    }

    #[cfg(test)]
    pub(crate) fn field_value_string(&self, flow_ref: TierFlowRef, field: &str) -> Option<String> {
        let normalized = field.to_ascii_uppercase();
        let index = self.indexes.get(&flow_ref.hour_start_usec)?;
        let field_ids = index.flow_field_ids(flow_ref.flow_id)?;

        match normalized.as_str() {
            "EXPORTER_IP" => {
                let present = rollup_field_value(index, field_ids, INTERNAL_EXPORTER_IP_PRESENT)
                    .is_some_and(|value| matches!(value, IndexFieldValue::U8(1)));
                if !present {
                    return Some(String::new());
                }
                rollup_field_value(index, field_ids, "EXPORTER_IP")
                    .map(compact_index_value_to_string)
            }
            "NEXT_HOP" => {
                let present = rollup_field_value(index, field_ids, INTERNAL_NEXT_HOP_PRESENT)
                    .is_some_and(|value| matches!(value, IndexFieldValue::U8(1)));
                if !present {
                    return Some(String::new());
                }
                rollup_field_value(index, field_ids, "NEXT_HOP").map(compact_index_value_to_string)
            }
            "DIRECTION" => {
                let present = rollup_field_value(index, field_ids, INTERNAL_DIRECTION_PRESENT)
                    .is_some_and(|value| matches!(value, IndexFieldValue::U8(1)));
                if !present {
                    return Some(String::new());
                }
                let value = rollup_field_value(index, field_ids, "DIRECTION")?;
                match value {
                    IndexFieldValue::U8(direction) => {
                        Some(direction_from_u8(direction).as_str().to_string())
                    }
                    _ => None,
                }
            }
            _ => {
                if let Some(internal_field) = rollup_presence_field(normalized.as_str()) {
                    let present = rollup_field_value(index, field_ids, internal_field)
                        .is_some_and(|value| matches!(value, IndexFieldValue::U8(1)));
                    if !present {
                        return Some(String::new());
                    }
                }
                rollup_field_value(index, field_ids, normalized.as_str())
                    .map(compact_index_value_to_string)
            }
        }
    }

    pub(crate) fn prune_unused_hours(&mut self, active_hours: &BTreeSet<u64>) {
        let before = self.indexes.len();
        self.indexes
            .retain(|hour_start_usec, _| active_hours.contains(hour_start_usec));
        if self.indexes.len() != before {
            self.generation = self.generation.saturating_add(1);
        }
    }
}
