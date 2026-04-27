use super::super::model::TierFlowRef;
use super::super::rollup::{
    HOUR_BUCKET_USEC, bucket_start_usec, build_rollup_flow_index, emit_rollup_row,
    materialize_rollup_fields, push_rollup_field_ids,
};
#[cfg(test)]
use super::super::rollup::{
    INTERNAL_DIRECTION_PRESENT, INTERNAL_EXPORTER_IP_PRESENT, INTERNAL_NEXT_HOP_PRESENT,
    compact_index_value_to_string, direction_from_u8, rollup_field_value, rollup_presence_field,
};
use crate::facet_runtime::FacetFileContribution;
use crate::flow::{FlowFields, FlowRecord};
#[cfg(test)]
use crate::flow_index::FieldValue as IndexFieldValue;
use crate::flow_index::{FlowIndex, FlowIndexMemoryBreakdown};
use crate::ingest::JournalEncodeBuffer;
use crate::tiering::FlowMetrics;
use anyhow::{Context, Result, anyhow};
use std::collections::{BTreeMap, BTreeSet};
use std::mem::size_of;

#[derive(Default)]
pub(crate) struct TierFlowIndexStore {
    generation: u64,
    indexes: BTreeMap<u64, FlowIndex>,
    scratch_field_ids: Vec<u32>,
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub(crate) struct TierFlowIndexMemoryBreakdown {
    pub(crate) index_keys_bytes: usize,
    pub(crate) schema_bytes: usize,
    pub(crate) field_store_bytes: usize,
    pub(crate) flow_lookup_bytes: usize,
    pub(crate) row_storage_bytes: usize,
    pub(crate) scratch_field_ids_bytes: usize,
}

impl TierFlowIndexMemoryBreakdown {
    pub(crate) fn total(self) -> usize {
        self.index_keys_bytes
            .saturating_add(self.schema_bytes)
            .saturating_add(self.field_store_bytes)
            .saturating_add(self.flow_lookup_bytes)
            .saturating_add(self.row_storage_bytes)
            .saturating_add(self.scratch_field_ids_bytes)
    }
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

        let field_ids_hash = index
            .hash_field_ids(&self.scratch_field_ids)
            .context("failed to hash tier flow dimensions")?;

        let flow_id = if let Some(existing) = index
            .find_flow_by_field_ids_hashed(&self.scratch_field_ids, field_ids_hash)
            .context("failed to find tier flow dimensions")?
        {
            existing
        } else {
            index
                .insert_flow_by_field_ids_hashed(&self.scratch_field_ids, field_ids_hash)
                .context("failed to insert tier flow dimensions")?
        };

        Ok(TierFlowRef {
            hour_start_usec,
            flow_id,
        })
    }

    #[allow(dead_code)]
    pub(crate) fn materialize_fields(&self, flow_ref: TierFlowRef) -> Option<FlowFields> {
        let index = self.indexes.get(&flow_ref.hour_start_usec)?;
        materialize_rollup_fields(index, flow_ref.flow_id)
    }

    pub(crate) fn emit_row(
        &self,
        flow_ref: TierFlowRef,
        metrics: FlowMetrics,
        encode_buf: &mut JournalEncodeBuffer,
    ) -> Option<FacetFileContribution> {
        let index = self.indexes.get(&flow_ref.hour_start_usec)?;
        emit_rollup_row(index, flow_ref.flow_id, metrics, encode_buf)
    }

    #[cfg(test)]
    pub(crate) fn index_for_test(&self, flow_ref: TierFlowRef) -> Option<&FlowIndex> {
        self.indexes.get(&flow_ref.hour_start_usec)
    }

    #[cfg(test)]
    pub(crate) fn field_value_string(&self, flow_ref: TierFlowRef, field: &str) -> Option<String> {
        let normalized = field.to_ascii_uppercase();
        let index = self.indexes.get(&flow_ref.hour_start_usec)?;

        match normalized.as_str() {
            "EXPORTER_IP" => {
                let present =
                    rollup_field_value(index, flow_ref.flow_id, INTERNAL_EXPORTER_IP_PRESENT)
                        .is_some_and(|value| matches!(value, IndexFieldValue::U8(1)));
                if !present {
                    return Some(String::new());
                }
                rollup_field_value(index, flow_ref.flow_id, "EXPORTER_IP")
                    .map(compact_index_value_to_string)
            }
            "NEXT_HOP" => {
                let present =
                    rollup_field_value(index, flow_ref.flow_id, INTERNAL_NEXT_HOP_PRESENT)
                        .is_some_and(|value| matches!(value, IndexFieldValue::U8(1)));
                if !present {
                    return Some(String::new());
                }
                rollup_field_value(index, flow_ref.flow_id, "NEXT_HOP")
                    .map(compact_index_value_to_string)
            }
            "DIRECTION" => {
                let present =
                    rollup_field_value(index, flow_ref.flow_id, INTERNAL_DIRECTION_PRESENT)
                        .is_some_and(|value| matches!(value, IndexFieldValue::U8(1)));
                if !present {
                    return Some(String::new());
                }
                let value = rollup_field_value(index, flow_ref.flow_id, "DIRECTION")?;
                match value {
                    IndexFieldValue::U8(direction) => {
                        Some(direction_from_u8(direction).as_str().to_string())
                    }
                    _ => None,
                }
            }
            _ => {
                if let Some(internal_field) = rollup_presence_field(normalized.as_str()) {
                    let present = rollup_field_value(index, flow_ref.flow_id, internal_field)
                        .is_some_and(|value| matches!(value, IndexFieldValue::U8(1)));
                    if !present {
                        return Some(String::new());
                    }
                }
                rollup_field_value(index, flow_ref.flow_id, normalized.as_str())
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

    pub(crate) fn estimated_heap_bytes(&self) -> usize {
        self.estimated_memory_breakdown().total()
    }

    pub(crate) fn estimated_memory_breakdown(&self) -> TierFlowIndexMemoryBreakdown {
        let mut breakdown = TierFlowIndexMemoryBreakdown {
            index_keys_bytes: self.indexes.len() * size_of::<u64>(),
            scratch_field_ids_bytes: self.scratch_field_ids.capacity() * size_of::<u32>(),
            ..TierFlowIndexMemoryBreakdown::default()
        };

        for index in self.indexes.values() {
            let FlowIndexMemoryBreakdown {
                schema_bytes,
                field_store_bytes,
                flow_lookup_bytes,
                row_storage_bytes,
            } = index.estimated_memory_breakdown();
            breakdown.schema_bytes = breakdown.schema_bytes.saturating_add(schema_bytes);
            breakdown.field_store_bytes = breakdown
                .field_store_bytes
                .saturating_add(field_store_bytes);
            breakdown.flow_lookup_bytes = breakdown
                .flow_lookup_bytes
                .saturating_add(flow_lookup_bytes);
            breakdown.row_storage_bytes = breakdown
                .row_storage_bytes
                .saturating_add(row_storage_bytes);
        }

        breakdown
    }
}
