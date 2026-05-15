use std::hash::Hasher;
use std::mem::size_of;
use std::net::{IpAddr, Ipv4Addr};

use super::{FieldId, FieldValue, FlowId, FlowIndexError, HashStrategy};

pub(super) enum FlowStorage {
    Dense(DenseFlowStorage),
    Sparse(SparseFlowStorage),
}

impl FlowStorage {
    pub(crate) fn dense(arity: usize) -> Self {
        Self::Dense(DenseFlowStorage::new(arity))
    }

    pub(crate) fn sparse_with_implicit_defaults(
        arity: usize,
        default_field_ids: Box<[FieldId]>,
    ) -> Result<Self, FlowIndexError> {
        Ok(Self::Sparse(SparseFlowStorage::new(
            arity,
            default_field_ids,
        )?))
    }

    pub(crate) fn flow_count(&self) -> usize {
        match self {
            Self::Dense(storage) => storage.flow_count(),
            Self::Sparse(storage) => storage.flow_count(),
        }
    }

    pub(crate) fn push_row(&mut self, field_ids: &[FieldId]) -> Result<(), FlowIndexError> {
        match self {
            Self::Dense(storage) => storage.push_row(field_ids),
            Self::Sparse(storage) => storage.push_row(field_ids),
        }
    }

    pub(crate) fn row_matches(&self, flow_id: FlowId, field_ids: &[FieldId]) -> bool {
        match self {
            Self::Dense(storage) => storage.row_matches(flow_id, field_ids),
            Self::Sparse(storage) => storage.row_matches(flow_id, field_ids),
        }
    }

    pub(crate) fn row_hash<H: HashStrategy>(&self, flow_id: FlowId, hasher: &H) -> Option<u64> {
        match self {
            Self::Dense(storage) => storage.row_hash(flow_id, hasher),
            Self::Sparse(storage) => storage.row_hash(flow_id, hasher),
        }
    }

    pub(crate) fn field_id(&self, flow_id: FlowId, field_index: usize) -> Option<FieldId> {
        match self {
            Self::Dense(storage) => storage.field_id(flow_id, field_index),
            Self::Sparse(storage) => storage.field_id(flow_id, field_index),
        }
    }

    pub(crate) fn estimated_heap_bytes(&self) -> usize {
        match self {
            Self::Dense(storage) => storage.estimated_heap_bytes(),
            Self::Sparse(storage) => storage.estimated_heap_bytes(),
        }
    }
}

pub(super) fn implicit_default_field_value(kind: super::FieldKind) -> FieldValue<'static> {
    match kind {
        super::FieldKind::Text => FieldValue::Text(""),
        super::FieldKind::U8 => FieldValue::U8(0),
        super::FieldKind::U16 => FieldValue::U16(0),
        super::FieldKind::U32 => FieldValue::U32(0),
        super::FieldKind::U64 => FieldValue::U64(0),
        super::FieldKind::IpAddr => FieldValue::IpAddr(IpAddr::V4(Ipv4Addr::UNSPECIFIED)),
    }
}

pub(super) struct DenseFlowStorage {
    arity: usize,
    values: Vec<FieldId>,
}

impl DenseFlowStorage {
    fn new(arity: usize) -> Self {
        Self {
            arity,
            values: Vec::new(),
        }
    }

    fn flow_count(&self) -> usize {
        if self.arity == 0 {
            0
        } else {
            self.values.len() / self.arity
        }
    }

    fn push_row(&mut self, field_ids: &[FieldId]) -> Result<(), FlowIndexError> {
        if field_ids.len() != self.arity {
            return Err(FlowIndexError::FieldCountMismatch {
                expected: self.arity,
                actual: field_ids.len(),
            });
        }
        self.values.extend_from_slice(field_ids);
        Ok(())
    }

    fn row_matches(&self, flow_id: FlowId, field_ids: &[FieldId]) -> bool {
        self.row_slice(flow_id) == Some(field_ids)
    }

    fn row_hash<H: HashStrategy>(&self, flow_id: FlowId, hasher: &H) -> Option<u64> {
        Some(hasher.hash_u32_slice(self.row_slice(flow_id)?))
    }

    fn field_id(&self, flow_id: FlowId, field_index: usize) -> Option<FieldId> {
        self.row_slice(flow_id)?.get(field_index).copied()
    }

    fn row_slice(&self, flow_id: FlowId) -> Option<&[FieldId]> {
        let start = flow_id as usize * self.arity;
        let end = start + self.arity;
        self.values.get(start..end)
    }

    fn estimated_heap_bytes(&self) -> usize {
        self.values.capacity() * size_of::<FieldId>()
    }
}

pub(super) struct SparseFlowStorage {
    arity: usize,
    default_field_ids: Box<[FieldId]>,
    row_offsets: Vec<u32>,
    row_field_indexes: Vec<u8>,
    row_field_ids: Vec<FieldId>,
}

impl SparseFlowStorage {
    fn new(arity: usize, default_field_ids: Box<[FieldId]>) -> Result<Self, FlowIndexError> {
        if arity > u8::MAX as usize {
            return Err(FlowIndexError::SparseFieldIndexOverflow { field_count: arity });
        }
        if default_field_ids.len() != arity {
            return Err(FlowIndexError::FieldCountMismatch {
                expected: arity,
                actual: default_field_ids.len(),
            });
        }

        Ok(Self {
            arity,
            default_field_ids,
            row_offsets: vec![0],
            row_field_indexes: Vec::new(),
            row_field_ids: Vec::new(),
        })
    }

    fn flow_count(&self) -> usize {
        self.row_offsets.len().saturating_sub(1)
    }

    fn push_row(&mut self, field_ids: &[FieldId]) -> Result<(), FlowIndexError> {
        if field_ids.len() != self.arity {
            return Err(FlowIndexError::FieldCountMismatch {
                expected: self.arity,
                actual: field_ids.len(),
            });
        }

        for (field_index, &field_id) in field_ids.iter().enumerate() {
            if field_id == self.default_field_ids[field_index] {
                continue;
            }

            self.row_field_indexes.push(field_index as u8);
            self.row_field_ids.push(field_id);
        }

        let end = u32::try_from(self.row_field_ids.len())
            .map_err(|_| FlowIndexError::RowStorageOverflow)?;
        self.row_offsets.push(end);
        Ok(())
    }

    fn row_matches(&self, flow_id: FlowId, field_ids: &[FieldId]) -> bool {
        let Some((mut sparse_index, sparse_end)) = self.row_bounds(flow_id) else {
            return false;
        };

        for (field_index, &expected) in field_ids.iter().enumerate() {
            if sparse_index < sparse_end
                && self.row_field_indexes[sparse_index] as usize == field_index
            {
                if self.row_field_ids[sparse_index] != expected {
                    return false;
                }
                sparse_index += 1;
            } else if self.default_field_ids[field_index] != expected {
                return false;
            }
        }

        sparse_index == sparse_end
    }

    fn row_hash<H: HashStrategy>(&self, flow_id: FlowId, hasher: &H) -> Option<u64> {
        let (mut sparse_index, sparse_end) = self.row_bounds(flow_id)?;
        let mut state = hasher.build_hasher();

        for field_index in 0..self.arity {
            let field_id = if sparse_index < sparse_end
                && self.row_field_indexes[sparse_index] as usize == field_index
            {
                let field_id = self.row_field_ids[sparse_index];
                sparse_index += 1;
                field_id
            } else {
                self.default_field_ids[field_index]
            };
            state.write_u32(field_id);
        }

        Some(state.finish())
    }

    fn field_id(&self, flow_id: FlowId, field_index: usize) -> Option<FieldId> {
        if field_index >= self.arity {
            return None;
        }

        let (start, end) = self.row_bounds(flow_id)?;
        let key = field_index as u8;
        match self.row_field_indexes[start..end].binary_search(&key) {
            Ok(offset) => Some(self.row_field_ids[start + offset]),
            Err(_) => Some(self.default_field_ids[field_index]),
        }
    }

    fn row_bounds(&self, flow_id: FlowId) -> Option<(usize, usize)> {
        let start = *self.row_offsets.get(flow_id as usize)? as usize;
        let end = *self.row_offsets.get(flow_id as usize + 1)? as usize;
        Some((start, end))
    }

    fn estimated_heap_bytes(&self) -> usize {
        self.default_field_ids.len() * size_of::<FieldId>()
            + self.row_offsets.capacity() * size_of::<u32>()
            + self.row_field_indexes.capacity() * size_of::<u8>()
            + self.row_field_ids.capacity() * size_of::<FieldId>()
    }
}
