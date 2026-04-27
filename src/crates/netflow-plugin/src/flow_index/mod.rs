use std::collections::BTreeSet;
use std::hash::Hasher;
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr};

use hashbrown::HashTable;
use std::mem::size_of;
use storage::{FlowStorage, implicit_default_field_value};
use thiserror::Error;
use twox_hash::XxHash64;

mod storage;

pub type FieldId = u32;
pub type FlowId = u32;

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct FieldSpec {
    name: Box<str>,
    kind: FieldKind,
}

impl FieldSpec {
    pub fn new(name: impl Into<String>, kind: FieldKind) -> Self {
        Self {
            name: name.into().into_boxed_str(),
            kind,
        }
    }

    pub fn name(&self) -> &str {
        &self.name
    }

    pub fn kind(&self) -> FieldKind {
        self.kind
    }
}

#[derive(Copy, Clone, Debug, PartialEq, Eq)]
pub enum FieldKind {
    Text,
    U8,
    U16,
    U32,
    U64,
    IpAddr,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub enum OwnedFieldValue {
    Text(Box<str>),
    U8(u8),
    U16(u16),
    U32(u32),
    U64(u64),
    IpAddr(IpAddr),
}

impl OwnedFieldValue {
    pub fn as_borrowed(&self) -> FieldValue<'_> {
        match self {
            Self::Text(value) => FieldValue::Text(value),
            Self::U8(value) => FieldValue::U8(*value),
            Self::U16(value) => FieldValue::U16(*value),
            Self::U32(value) => FieldValue::U32(*value),
            Self::U64(value) => FieldValue::U64(*value),
            Self::IpAddr(value) => FieldValue::IpAddr(*value),
        }
    }

    pub fn kind(&self) -> FieldKind {
        match self {
            Self::Text(_) => FieldKind::Text,
            Self::U8(_) => FieldKind::U8,
            Self::U16(_) => FieldKind::U16,
            Self::U32(_) => FieldKind::U32,
            Self::U64(_) => FieldKind::U64,
            Self::IpAddr(_) => FieldKind::IpAddr,
        }
    }
}

#[derive(Copy, Clone, Debug, PartialEq, Eq)]
pub enum FieldValue<'a> {
    Text(&'a str),
    U8(u8),
    U16(u16),
    U32(u32),
    U64(u64),
    IpAddr(IpAddr),
}

impl<'a> FieldValue<'a> {
    pub fn kind(self) -> FieldKind {
        match self {
            Self::Text(_) => FieldKind::Text,
            Self::U8(_) => FieldKind::U8,
            Self::U16(_) => FieldKind::U16,
            Self::U32(_) => FieldKind::U32,
            Self::U64(_) => FieldKind::U64,
            Self::IpAddr(_) => FieldKind::IpAddr,
        }
    }

    pub fn to_owned(self) -> OwnedFieldValue {
        match self {
            Self::Text(value) => OwnedFieldValue::Text(value.into()),
            Self::U8(value) => OwnedFieldValue::U8(value),
            Self::U16(value) => OwnedFieldValue::U16(value),
            Self::U32(value) => OwnedFieldValue::U32(value),
            Self::U64(value) => OwnedFieldValue::U64(value),
            Self::IpAddr(value) => OwnedFieldValue::IpAddr(value),
        }
    }
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct FlowSchema {
    fields: Box<[FieldSpec]>,
}

impl FlowSchema {
    pub fn new(fields: impl IntoIterator<Item = FieldSpec>) -> Result<Self, FlowIndexError> {
        let fields: Vec<FieldSpec> = fields.into_iter().collect();
        if fields.is_empty() {
            return Err(FlowIndexError::EmptySchema);
        }

        let mut seen = BTreeSet::new();
        for field in &fields {
            if !seen.insert(field.name().to_string()) {
                return Err(FlowIndexError::DuplicateFieldName(field.name().to_string()));
            }
        }

        Ok(Self {
            fields: fields.into_boxed_slice(),
        })
    }

    pub fn len(&self) -> usize {
        self.fields.len()
    }

    pub fn is_empty(&self) -> bool {
        self.fields.is_empty()
    }

    pub fn field(&self, index: usize) -> Option<&FieldSpec> {
        self.fields.get(index)
    }

    pub fn fields(&self) -> &[FieldSpec] {
        &self.fields
    }

    pub fn estimated_heap_bytes(&self) -> usize {
        self.fields.len() * size_of::<FieldSpec>()
            + self
                .fields
                .iter()
                .map(|field| field.name.len())
                .sum::<usize>()
    }
}

#[derive(Debug, Error, PartialEq, Eq)]
pub enum FlowIndexError {
    #[error("schema must contain at least one field")]
    EmptySchema,
    #[error("duplicate field name `{0}` in schema")]
    DuplicateFieldName(String),
    #[error("field count mismatch: expected {expected}, got {actual}")]
    FieldCountMismatch { expected: usize, actual: usize },
    #[error("invalid field index {field_index}")]
    InvalidFieldIndex { field_index: usize },
    #[error("field `{field_name}` expects `{expected:?}` but got `{actual:?}`")]
    FieldKindMismatch {
        field_name: String,
        expected: FieldKind,
        actual: FieldKind,
    },
    #[error("field id overflow")]
    FieldIdOverflow,
    #[error("flow id overflow")]
    FlowIdOverflow,
    #[error("text arena overflow")]
    TextArenaOverflow,
    #[error("sparse flow storage supports at most 255 fields, got {field_count}")]
    SparseFieldIndexOverflow { field_count: usize },
    #[error("row storage overflow")]
    RowStorageOverflow,
    #[error("failed to parse field `{field_name}` as `{kind:?}` from `{value}`")]
    Parse {
        field_name: String,
        kind: FieldKind,
        value: String,
    },
}

pub trait HashStrategy: Clone {
    type Hasher: Hasher;

    fn build_hasher(&self) -> Self::Hasher;

    fn hash_bytes(&self, bytes: &[u8]) -> u64 {
        let mut hasher = self.build_hasher();
        hasher.write(bytes);
        hasher.finish()
    }

    fn hash_u8(&self, value: u8) -> u64 {
        let mut hasher = self.build_hasher();
        hasher.write_u8(value);
        hasher.finish()
    }

    fn hash_u16(&self, value: u16) -> u64 {
        let mut hasher = self.build_hasher();
        hasher.write_u16(value);
        hasher.finish()
    }

    fn hash_u32(&self, value: u32) -> u64 {
        let mut hasher = self.build_hasher();
        hasher.write_u32(value);
        hasher.finish()
    }

    fn hash_u64(&self, value: u64) -> u64 {
        let mut hasher = self.build_hasher();
        hasher.write_u64(value);
        hasher.finish()
    }

    fn hash_ip_parts(&self, family: u8, bytes: &[u8; 16]) -> u64 {
        let mut hasher = self.build_hasher();
        hasher.write_u8(family);
        hasher.write(bytes);
        hasher.finish()
    }

    fn hash_u32_slice(&self, values: &[u32]) -> u64 {
        let mut hasher = self.build_hasher();
        for value in values {
            hasher.write_u32(*value);
        }
        hasher.finish()
    }
}

#[derive(Clone, Default)]
pub struct XxHash64Strategy;

impl HashStrategy for XxHash64Strategy {
    type Hasher = XxHash64;

    fn build_hasher(&self) -> Self::Hasher {
        XxHash64::default()
    }
}

pub struct FlowIndex<H = XxHash64Strategy>
where
    H: HashStrategy,
{
    schema: FlowSchema,
    field_stores: Box<[FieldStore]>,
    flow_lookup: HashTable<u32>,
    flow_storage: FlowStorage,
    hasher: H,
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct FlowIndexMemoryBreakdown {
    pub schema_bytes: usize,
    pub field_store_bytes: usize,
    pub flow_lookup_bytes: usize,
    pub row_storage_bytes: usize,
}

impl FlowIndexMemoryBreakdown {
    pub fn total(self) -> usize {
        self.schema_bytes
            .saturating_add(self.field_store_bytes)
            .saturating_add(self.flow_lookup_bytes)
            .saturating_add(self.row_storage_bytes)
    }
}

impl FlowIndex<XxHash64Strategy> {
    pub fn new(fields: impl IntoIterator<Item = FieldSpec>) -> Result<Self, FlowIndexError> {
        Self::with_hasher(fields, XxHash64Strategy)
    }

    pub fn new_with_implicit_defaults(
        fields: impl IntoIterator<Item = FieldSpec>,
    ) -> Result<Self, FlowIndexError> {
        Self::with_hasher_and_implicit_defaults(fields, XxHash64Strategy)
    }
}

impl<H> FlowIndex<H>
where
    H: HashStrategy,
{
    pub fn with_hasher(
        fields: impl IntoIterator<Item = FieldSpec>,
        hasher: H,
    ) -> Result<Self, FlowIndexError> {
        Self::build(fields, hasher, false)
    }

    pub fn with_hasher_and_implicit_defaults(
        fields: impl IntoIterator<Item = FieldSpec>,
        hasher: H,
    ) -> Result<Self, FlowIndexError> {
        Self::build(fields, hasher, true)
    }

    fn build(
        fields: impl IntoIterator<Item = FieldSpec>,
        hasher: H,
        implicit_defaults: bool,
    ) -> Result<Self, FlowIndexError> {
        let schema = FlowSchema::new(fields)?;
        let mut field_stores = schema
            .fields()
            .iter()
            .map(|field| FieldStore::new(field.kind()))
            .collect::<Vec<_>>()
            .into_boxed_slice();
        let flow_storage = if implicit_defaults {
            let default_field_ids = schema
                .fields()
                .iter()
                .enumerate()
                .map(|(field_index, field)| {
                    field_stores[field_index]
                        .get_or_insert(implicit_default_field_value(field.kind()), &hasher)
                })
                .collect::<Result<Vec<_>, _>>()?
                .into_boxed_slice();
            FlowStorage::sparse_with_implicit_defaults(schema.len(), default_field_ids)?
        } else {
            FlowStorage::dense(schema.len())
        };

        Ok(Self {
            schema,
            field_stores,
            flow_lookup: HashTable::new(),
            flow_storage,
            hasher,
        })
    }

    pub fn schema(&self) -> &FlowSchema {
        &self.schema
    }

    pub fn flow_count(&self) -> usize {
        self.flow_storage.flow_count()
    }

    pub fn get_or_insert_parsed_field_value(
        &mut self,
        field_index: usize,
        raw: &str,
    ) -> Result<FieldId, FlowIndexError> {
        let field = self
            .schema
            .field(field_index)
            .ok_or(FlowIndexError::InvalidFieldIndex { field_index })?;
        let parsed = parse_field_value(field, raw)?;
        self.get_or_insert_field_value(field_index, parsed.as_borrowed())
    }

    pub fn get_or_insert_field_value(
        &mut self,
        field_index: usize,
        value: FieldValue<'_>,
    ) -> Result<FieldId, FlowIndexError> {
        let field = self
            .schema
            .field(field_index)
            .ok_or(FlowIndexError::InvalidFieldIndex { field_index })?;
        if value.kind() != field.kind() {
            return Err(FlowIndexError::FieldKindMismatch {
                field_name: field.name().to_string(),
                expected: field.kind(),
                actual: value.kind(),
            });
        }

        self.field_stores[field_index].get_or_insert(value, &self.hasher)
    }

    pub fn find_field_value(
        &self,
        field_index: usize,
        value: FieldValue<'_>,
    ) -> Result<Option<FieldId>, FlowIndexError> {
        let field = self
            .schema
            .field(field_index)
            .ok_or(FlowIndexError::InvalidFieldIndex { field_index })?;
        if value.kind() != field.kind() {
            return Err(FlowIndexError::FieldKindMismatch {
                field_name: field.name().to_string(),
                expected: field.kind(),
                actual: value.kind(),
            });
        }

        Ok(self.field_stores[field_index].find(value, &self.hasher))
    }

    pub fn field_value(&self, field_index: usize, field_id: FieldId) -> Option<FieldValue<'_>> {
        self.field_stores.get(field_index)?.value(field_id)
    }

    pub fn get_or_insert_parsed_flow(
        &mut self,
        raw_values: &[&str],
    ) -> Result<FlowId, FlowIndexError> {
        if raw_values.len() != self.schema.len() {
            return Err(FlowIndexError::FieldCountMismatch {
                expected: self.schema.len(),
                actual: raw_values.len(),
            });
        }

        let mut values = Vec::with_capacity(raw_values.len());
        for (index, raw) in raw_values.iter().enumerate() {
            let field = self
                .schema
                .field(index)
                .ok_or(FlowIndexError::InvalidFieldIndex { field_index: index })?;
            values.push(parse_field_value(field, raw)?);
        }

        let borrowed = values
            .iter()
            .map(OwnedFieldValue::as_borrowed)
            .collect::<Vec<_>>();
        self.get_or_insert_flow(&borrowed)
    }

    pub fn get_or_insert_flow(
        &mut self,
        values: &[FieldValue<'_>],
    ) -> Result<FlowId, FlowIndexError> {
        if values.len() != self.schema.len() {
            return Err(FlowIndexError::FieldCountMismatch {
                expected: self.schema.len(),
                actual: values.len(),
            });
        }

        let mut field_ids = Vec::with_capacity(values.len());
        for (index, value) in values.iter().copied().enumerate() {
            field_ids.push(self.get_or_insert_field_value(index, value)?);
        }

        let hash = self.hash_field_ids(&field_ids)?;
        if let Some(existing_id) = self.find_flow_by_field_ids_hashed(&field_ids, hash)? {
            return Ok(existing_id);
        }

        self.insert_flow_by_field_ids_hashed(&field_ids, hash)
    }

    pub fn find_flow(&self, values: &[FieldValue<'_>]) -> Result<Option<FlowId>, FlowIndexError> {
        if values.len() != self.schema.len() {
            return Err(FlowIndexError::FieldCountMismatch {
                expected: self.schema.len(),
                actual: values.len(),
            });
        }

        let mut field_ids = Vec::with_capacity(values.len());
        for (index, value) in values.iter().copied().enumerate() {
            let Some(field_id) = self.find_field_value(index, value)? else {
                return Ok(None);
            };
            field_ids.push(field_id);
        }

        self.find_flow_by_field_ids(&field_ids)
    }

    pub fn find_flow_by_field_ids(
        &self,
        field_ids: &[FieldId],
    ) -> Result<Option<FlowId>, FlowIndexError> {
        let hash = self.hash_field_ids(field_ids)?;
        self.find_flow_by_field_ids_hashed(field_ids, hash)
    }

    pub fn find_flow_by_field_ids_hashed(
        &self,
        field_ids: &[FieldId],
        hash: u64,
    ) -> Result<Option<FlowId>, FlowIndexError> {
        if field_ids.len() != self.schema.len() {
            return Err(FlowIndexError::FieldCountMismatch {
                expected: self.schema.len(),
                actual: field_ids.len(),
            });
        }

        Ok(self
            .flow_lookup
            .find(hash, |flow_id| {
                self.flow_storage.row_matches(*flow_id, field_ids)
            })
            .copied())
    }

    pub fn insert_flow_by_field_ids(
        &mut self,
        field_ids: &[FieldId],
    ) -> Result<FlowId, FlowIndexError> {
        let hash = self.hash_field_ids(field_ids)?;
        self.insert_flow_by_field_ids_hashed(field_ids, hash)
    }

    pub fn insert_flow_by_field_ids_hashed(
        &mut self,
        field_ids: &[FieldId],
        hash: u64,
    ) -> Result<FlowId, FlowIndexError> {
        if field_ids.len() != self.schema.len() {
            return Err(FlowIndexError::FieldCountMismatch {
                expected: self.schema.len(),
                actual: field_ids.len(),
            });
        }

        let flow_id = next_flow_id(self.flow_count())?;
        self.flow_storage.push_row(field_ids)?;

        let flow_storage = &self.flow_storage;
        let hasher = self.hasher.clone();
        self.flow_lookup
            .insert_unique(hash, flow_id, move |existing_id| {
                flow_storage
                    .row_hash(*existing_id, &hasher)
                    .expect("stored flow should hash successfully")
            });

        Ok(flow_id)
    }

    pub fn hash_field_ids(&self, field_ids: &[FieldId]) -> Result<u64, FlowIndexError> {
        if field_ids.len() != self.schema.len() {
            return Err(FlowIndexError::FieldCountMismatch {
                expected: self.schema.len(),
                actual: field_ids.len(),
            });
        }

        Ok(self.hasher.hash_u32_slice(field_ids))
    }

    pub fn flow_field_id(&self, flow_id: FlowId, field_index: usize) -> Option<FieldId> {
        self.flow_storage.field_id(flow_id, field_index)
    }

    pub fn flow_fields(&self, flow_id: FlowId) -> Option<Vec<OwnedFieldValue>> {
        let mut values = Vec::with_capacity(self.schema.len());
        for index in 0..self.schema.len() {
            let field_id = self.flow_field_id(flow_id, index)?;
            values.push(self.field_stores[index].owned_value(field_id)?);
        }
        Some(values)
    }

    pub fn estimated_heap_bytes(&self) -> usize {
        self.estimated_memory_breakdown().total()
    }

    pub fn estimated_memory_breakdown(&self) -> FlowIndexMemoryBreakdown {
        FlowIndexMemoryBreakdown {
            schema_bytes: self.schema.estimated_heap_bytes(),
            field_store_bytes: self
                .field_stores
                .iter()
                .map(FieldStore::estimated_heap_bytes)
                .sum::<usize>(),
            flow_lookup_bytes: hash_table_allocation_bytes(
                self.flow_lookup.capacity(),
                size_of::<u32>(),
            ),
            row_storage_bytes: self.flow_storage.estimated_heap_bytes(),
        }
    }
}

fn parse_field_value(field: &FieldSpec, raw: &str) -> Result<OwnedFieldValue, FlowIndexError> {
    match field.kind() {
        FieldKind::Text => Ok(OwnedFieldValue::Text(raw.into())),
        FieldKind::U8 => raw
            .parse()
            .map(OwnedFieldValue::U8)
            .map_err(|_| FlowIndexError::Parse {
                field_name: field.name().to_string(),
                kind: field.kind(),
                value: raw.to_string(),
            }),
        FieldKind::U16 => {
            raw.parse()
                .map(OwnedFieldValue::U16)
                .map_err(|_| FlowIndexError::Parse {
                    field_name: field.name().to_string(),
                    kind: field.kind(),
                    value: raw.to_string(),
                })
        }
        FieldKind::U32 => {
            raw.parse()
                .map(OwnedFieldValue::U32)
                .map_err(|_| FlowIndexError::Parse {
                    field_name: field.name().to_string(),
                    kind: field.kind(),
                    value: raw.to_string(),
                })
        }
        FieldKind::U64 => {
            raw.parse()
                .map(OwnedFieldValue::U64)
                .map_err(|_| FlowIndexError::Parse {
                    field_name: field.name().to_string(),
                    kind: field.kind(),
                    value: raw.to_string(),
                })
        }
        FieldKind::IpAddr => {
            raw.parse()
                .map(OwnedFieldValue::IpAddr)
                .map_err(|_| FlowIndexError::Parse {
                    field_name: field.name().to_string(),
                    kind: field.kind(),
                    value: raw.to_string(),
                })
        }
    }
}

fn next_field_id(len: usize) -> Result<FieldId, FlowIndexError> {
    u32::try_from(len).map_err(|_| FlowIndexError::FieldIdOverflow)
}

fn next_flow_id(len: usize) -> Result<FlowId, FlowIndexError> {
    u32::try_from(len).map_err(|_| FlowIndexError::FlowIdOverflow)
}

#[derive(Copy, Clone, Debug, PartialEq, Eq)]
struct TextEntry {
    offset: u32,
    len: u32,
}

const V6_FIELD_ID_TAG: FieldId = 1 << 31;

#[derive(Copy, Clone, Debug, PartialEq, Eq)]
enum CanonicalIpAddr {
    V4(u32),
    V6([u8; 16]),
}

impl CanonicalIpAddr {
    fn from_ip(value: IpAddr) -> Self {
        match value {
            IpAddr::V4(ip) => Self::V4(u32::from_be_bytes(ip.octets())),
            IpAddr::V6(ip) => ip
                .to_ipv4_mapped()
                .map(|mapped| Self::V4(u32::from_be_bytes(mapped.octets())))
                .unwrap_or_else(|| Self::V6(ip.octets())),
        }
    }

    fn into_ip(self) -> IpAddr {
        match self {
            Self::V4(bits) => IpAddr::V4(Ipv4Addr::from(bits.to_be_bytes())),
            Self::V6(bytes) => IpAddr::V6(Ipv6Addr::from(bytes)),
        }
    }
}

fn next_ip_field_id(len: usize, v6: bool) -> Result<FieldId, FlowIndexError> {
    let raw = u32::try_from(len).map_err(|_| FlowIndexError::FieldIdOverflow)?;
    if raw >= V6_FIELD_ID_TAG {
        return Err(FlowIndexError::FieldIdOverflow);
    }
    Ok(if v6 { raw | V6_FIELD_ID_TAG } else { raw })
}

fn decode_ip_field_id(field_id: FieldId) -> (bool, usize) {
    let is_v6 = (field_id & V6_FIELD_ID_TAG) != 0;
    let raw = field_id & !V6_FIELD_ID_TAG;
    (is_v6, raw as usize)
}

struct TextFieldStore {
    lookup: HashTable<u32>,
    entries: Vec<TextEntry>,
    arena: Vec<u8>,
}

impl TextFieldStore {
    fn new() -> Self {
        Self {
            lookup: HashTable::new(),
            entries: Vec::new(),
            arena: Vec::new(),
        }
    }

    fn get_or_insert<H: HashStrategy>(
        &mut self,
        value: &str,
        hasher: &H,
    ) -> Result<FieldId, FlowIndexError> {
        let hash = hasher.hash_bytes(value.as_bytes());
        if let Some(existing_id) = self
            .lookup
            .find(hash, |field_id| self.value_str(*field_id) == Some(value))
            .copied()
        {
            return Ok(existing_id);
        }

        let field_id = next_field_id(self.entries.len())?;
        let offset =
            u32::try_from(self.arena.len()).map_err(|_| FlowIndexError::TextArenaOverflow)?;
        let len = u32::try_from(value.len()).map_err(|_| FlowIndexError::TextArenaOverflow)?;

        self.arena.extend_from_slice(value.as_bytes());
        self.entries.push(TextEntry { offset, len });

        let entries = &self.entries;
        let arena = &self.arena;
        let hasher = hasher.clone();
        self.lookup
            .insert_unique(hash, field_id, move |existing_id| {
                let entry = entries[*existing_id as usize];
                let start = entry.offset as usize;
                let end = start + entry.len as usize;
                hasher.hash_bytes(&arena[start..end])
            });

        Ok(field_id)
    }

    fn find<H: HashStrategy>(&self, value: &str, hasher: &H) -> Option<FieldId> {
        let hash = hasher.hash_bytes(value.as_bytes());
        self.lookup
            .find(hash, |field_id| self.value_str(*field_id) == Some(value))
            .copied()
    }

    fn value_str(&self, field_id: FieldId) -> Option<&str> {
        let entry = self.entries.get(field_id as usize)?;
        let start = entry.offset as usize;
        let end = start + entry.len as usize;
        std::str::from_utf8(self.arena.get(start..end)?).ok()
    }

    fn value(&self, field_id: FieldId) -> Option<FieldValue<'_>> {
        Some(FieldValue::Text(self.value_str(field_id)?))
    }

    fn owned_value(&self, field_id: FieldId) -> Option<OwnedFieldValue> {
        Some(OwnedFieldValue::Text(self.value_str(field_id)?.into()))
    }

    fn estimated_heap_bytes(&self) -> usize {
        hash_table_allocation_bytes(self.lookup.capacity(), size_of::<u32>())
            + self.entries.capacity() * size_of::<TextEntry>()
            + self.arena.capacity()
    }
}

macro_rules! define_numeric_store {
    ($name:ident, $ty:ty, $variant:ident, $hash_method:ident) => {
        struct $name {
            lookup: HashTable<u32>,
            values: Vec<$ty>,
        }

        impl $name {
            fn new() -> Self {
                Self {
                    lookup: HashTable::new(),
                    values: Vec::new(),
                }
            }

            fn get_or_insert<H: HashStrategy>(
                &mut self,
                value: $ty,
                hasher: &H,
            ) -> Result<FieldId, FlowIndexError> {
                let hash = hasher.$hash_method(value);
                if let Some(existing_id) = self
                    .lookup
                    .find(hash, |field_id| self.values[*field_id as usize] == value)
                    .copied()
                {
                    return Ok(existing_id);
                }

                let field_id = next_field_id(self.values.len())?;
                self.values.push(value);

                let values = &self.values;
                let hasher = hasher.clone();
                self.lookup
                    .insert_unique(hash, field_id, move |existing_id| {
                        hasher.$hash_method(values[*existing_id as usize])
                    });

                Ok(field_id)
            }

            fn find<H: HashStrategy>(&self, value: $ty, hasher: &H) -> Option<FieldId> {
                let hash = hasher.$hash_method(value);
                self.lookup
                    .find(hash, |field_id| self.values[*field_id as usize] == value)
                    .copied()
            }

            fn value(&self, field_id: FieldId) -> Option<FieldValue<'_>> {
                Some(FieldValue::$variant(*self.values.get(field_id as usize)?))
            }

            fn owned_value(&self, field_id: FieldId) -> Option<OwnedFieldValue> {
                Some(OwnedFieldValue::$variant(
                    *self.values.get(field_id as usize)?,
                ))
            }

            fn estimated_heap_bytes(&self) -> usize {
                hash_table_allocation_bytes(self.lookup.capacity(), size_of::<u32>())
                    + self.values.capacity() * size_of::<$ty>()
            }
        }
    };
}

define_numeric_store!(U8FieldStore, u8, U8, hash_u8);
define_numeric_store!(U16FieldStore, u16, U16, hash_u16);
define_numeric_store!(U32FieldStore, u32, U32, hash_u32);
define_numeric_store!(U64FieldStore, u64, U64, hash_u64);

struct IpFieldStore {
    v4_lookup: HashTable<u32>,
    v4_values: Vec<u32>,
    v6_lookup: HashTable<u32>,
    v6_values: Vec<[u8; 16]>,
}

impl IpFieldStore {
    fn new() -> Self {
        Self {
            v4_lookup: HashTable::new(),
            v4_values: Vec::new(),
            v6_lookup: HashTable::new(),
            v6_values: Vec::new(),
        }
    }

    fn get_or_insert<H: HashStrategy>(
        &mut self,
        value: IpAddr,
        hasher: &H,
    ) -> Result<FieldId, FlowIndexError> {
        match CanonicalIpAddr::from_ip(value) {
            CanonicalIpAddr::V4(bits) => {
                let hash = hasher.hash_u32(bits);
                if let Some(existing_id) = self
                    .v4_lookup
                    .find(hash, |field_id| self.v4_values[*field_id as usize] == bits)
                    .copied()
                {
                    return Ok(existing_id);
                }

                let field_id = next_ip_field_id(self.v4_values.len(), false)?;
                self.v4_values.push(bits);

                let values = &self.v4_values;
                let hasher = hasher.clone();
                self.v4_lookup
                    .insert_unique(hash, field_id, move |existing_id| {
                        hasher.hash_u32(values[*existing_id as usize])
                    });

                Ok(field_id)
            }
            CanonicalIpAddr::V6(bytes) => {
                let hash = hasher.hash_ip_parts(6, &bytes);
                if let Some(existing_id) = self
                    .v6_lookup
                    .find(hash, |field_id| {
                        let (_, index) = decode_ip_field_id(*field_id);
                        self.v6_values[index] == bytes
                    })
                    .copied()
                {
                    return Ok(existing_id);
                }

                let field_id = next_ip_field_id(self.v6_values.len(), true)?;
                self.v6_values.push(bytes);

                let values = &self.v6_values;
                let hasher = hasher.clone();
                self.v6_lookup
                    .insert_unique(hash, field_id, move |existing_id| {
                        let (_, index) = decode_ip_field_id(*existing_id);
                        hasher.hash_ip_parts(6, &values[index])
                    });

                Ok(field_id)
            }
        }
    }

    fn find<H: HashStrategy>(&self, value: IpAddr, hasher: &H) -> Option<FieldId> {
        match CanonicalIpAddr::from_ip(value) {
            CanonicalIpAddr::V4(bits) => {
                let hash = hasher.hash_u32(bits);
                self.v4_lookup
                    .find(hash, |field_id| self.v4_values[*field_id as usize] == bits)
                    .copied()
            }
            CanonicalIpAddr::V6(bytes) => {
                let hash = hasher.hash_ip_parts(6, &bytes);
                self.v6_lookup
                    .find(hash, |field_id| {
                        let (_, index) = decode_ip_field_id(*field_id);
                        self.v6_values[index] == bytes
                    })
                    .copied()
            }
        }
    }

    fn value(&self, field_id: FieldId) -> Option<FieldValue<'_>> {
        Some(FieldValue::IpAddr(self.ip_value(field_id)?.into_ip()))
    }

    fn owned_value(&self, field_id: FieldId) -> Option<OwnedFieldValue> {
        Some(OwnedFieldValue::IpAddr(self.ip_value(field_id)?.into_ip()))
    }

    fn ip_value(&self, field_id: FieldId) -> Option<CanonicalIpAddr> {
        let (is_v6, index) = decode_ip_field_id(field_id);
        if is_v6 {
            self.v6_values.get(index).copied().map(CanonicalIpAddr::V6)
        } else {
            self.v4_values.get(index).copied().map(CanonicalIpAddr::V4)
        }
    }

    fn estimated_heap_bytes(&self) -> usize {
        hash_table_allocation_bytes(self.v4_lookup.capacity(), size_of::<u32>())
            + self.v4_values.capacity() * size_of::<u32>()
            + hash_table_allocation_bytes(self.v6_lookup.capacity(), size_of::<u32>())
            + self.v6_values.capacity() * size_of::<[u8; 16]>()
    }
}

enum FieldStore {
    Text(TextFieldStore),
    U8(U8FieldStore),
    U16(U16FieldStore),
    U32(U32FieldStore),
    U64(U64FieldStore),
    IpAddr(IpFieldStore),
}

impl FieldStore {
    fn new(kind: FieldKind) -> Self {
        match kind {
            FieldKind::Text => Self::Text(TextFieldStore::new()),
            FieldKind::U8 => Self::U8(U8FieldStore::new()),
            FieldKind::U16 => Self::U16(U16FieldStore::new()),
            FieldKind::U32 => Self::U32(U32FieldStore::new()),
            FieldKind::U64 => Self::U64(U64FieldStore::new()),
            FieldKind::IpAddr => Self::IpAddr(IpFieldStore::new()),
        }
    }

    fn get_or_insert<H: HashStrategy>(
        &mut self,
        value: FieldValue<'_>,
        hasher: &H,
    ) -> Result<FieldId, FlowIndexError> {
        match (self, value) {
            (Self::Text(store), FieldValue::Text(value)) => store.get_or_insert(value, hasher),
            (Self::U8(store), FieldValue::U8(value)) => store.get_or_insert(value, hasher),
            (Self::U16(store), FieldValue::U16(value)) => store.get_or_insert(value, hasher),
            (Self::U32(store), FieldValue::U32(value)) => store.get_or_insert(value, hasher),
            (Self::U64(store), FieldValue::U64(value)) => store.get_or_insert(value, hasher),
            (Self::IpAddr(store), FieldValue::IpAddr(value)) => store.get_or_insert(value, hasher),
            _ => unreachable!("field kind mismatch is validated by caller"),
        }
    }

    fn find<H: HashStrategy>(&self, value: FieldValue<'_>, hasher: &H) -> Option<FieldId> {
        match (self, value) {
            (Self::Text(store), FieldValue::Text(value)) => store.find(value, hasher),
            (Self::U8(store), FieldValue::U8(value)) => store.find(value, hasher),
            (Self::U16(store), FieldValue::U16(value)) => store.find(value, hasher),
            (Self::U32(store), FieldValue::U32(value)) => store.find(value, hasher),
            (Self::U64(store), FieldValue::U64(value)) => store.find(value, hasher),
            (Self::IpAddr(store), FieldValue::IpAddr(value)) => store.find(value, hasher),
            _ => unreachable!("field kind mismatch is validated by caller"),
        }
    }

    fn value(&self, field_id: FieldId) -> Option<FieldValue<'_>> {
        match self {
            Self::Text(store) => store.value(field_id),
            Self::U8(store) => store.value(field_id),
            Self::U16(store) => store.value(field_id),
            Self::U32(store) => store.value(field_id),
            Self::U64(store) => store.value(field_id),
            Self::IpAddr(store) => store.value(field_id),
        }
    }

    fn owned_value(&self, field_id: FieldId) -> Option<OwnedFieldValue> {
        match self {
            Self::Text(store) => store.owned_value(field_id),
            Self::U8(store) => store.owned_value(field_id),
            Self::U16(store) => store.owned_value(field_id),
            Self::U32(store) => store.owned_value(field_id),
            Self::U64(store) => store.owned_value(field_id),
            Self::IpAddr(store) => store.owned_value(field_id),
        }
    }

    fn estimated_heap_bytes(&self) -> usize {
        match self {
            Self::Text(store) => store.estimated_heap_bytes(),
            Self::U8(store) => store.estimated_heap_bytes(),
            Self::U16(store) => store.estimated_heap_bytes(),
            Self::U32(store) => store.estimated_heap_bytes(),
            Self::U64(store) => store.estimated_heap_bytes(),
            Self::IpAddr(store) => store.estimated_heap_bytes(),
        }
    }
}

fn hash_table_allocation_bytes(capacity: usize, element_size: usize) -> usize {
    capacity.saturating_mul(element_size.saturating_add(1))
}

#[cfg(test)]
mod tests {
    use std::collections::BTreeSet;

    use proptest::prelude::*;

    use super::*;

    #[derive(Clone, Default)]
    struct ConstantHashStrategy;

    #[derive(Default)]
    struct ConstantHasher;

    impl Hasher for ConstantHasher {
        fn finish(&self) -> u64 {
            0
        }

        fn write(&mut self, _bytes: &[u8]) {}
    }

    impl HashStrategy for ConstantHashStrategy {
        type Hasher = ConstantHasher;

        fn build_hasher(&self) -> Self::Hasher {
            ConstantHasher
        }
    }

    #[test]
    fn schema_rejects_duplicate_names() {
        let error = FlowSchema::new([
            FieldSpec::new("SRC_ADDR", FieldKind::IpAddr),
            FieldSpec::new("SRC_ADDR", FieldKind::IpAddr),
        ])
        .expect_err("duplicate field names should fail");

        assert_eq!(
            error,
            FlowIndexError::DuplicateFieldName("SRC_ADDR".to_string())
        );
    }

    #[test]
    fn text_field_interning_reuses_ids_under_total_hash_collision() {
        let mut index = FlowIndex::with_hasher(
            [FieldSpec::new("SRC_AS_NAME", FieldKind::Text)],
            ConstantHashStrategy,
        )
        .expect("index");

        let a1 = index
            .get_or_insert_field_value(0, FieldValue::Text("CLOUDFLARE"))
            .expect("first string");
        let a2 = index
            .get_or_insert_field_value(0, FieldValue::Text("CLOUDFLARE"))
            .expect("same string");
        let b = index
            .get_or_insert_field_value(0, FieldValue::Text("ANTHROPIC"))
            .expect("different string");

        assert_eq!(a1, a2);
        assert_ne!(a1, b);
        assert_eq!(
            index.field_value(0, a1),
            Some(FieldValue::Text("CLOUDFLARE"))
        );
        assert_eq!(index.field_value(0, b), Some(FieldValue::Text("ANTHROPIC")));
    }

    #[test]
    fn flow_interning_reuses_ids_under_total_hash_collision() {
        let mut index = FlowIndex::with_hasher(
            [
                FieldSpec::new("SRC_AS_NAME", FieldKind::Text),
                FieldSpec::new("DST_PORT", FieldKind::U16),
            ],
            ConstantHashStrategy,
        )
        .expect("index");

        let flow_a1 = index
            .get_or_insert_flow(&[FieldValue::Text("A"), FieldValue::U16(443)])
            .expect("flow a1");
        let flow_b = index
            .get_or_insert_flow(&[FieldValue::Text("B"), FieldValue::U16(443)])
            .expect("flow b");
        let flow_a2 = index
            .get_or_insert_flow(&[FieldValue::Text("A"), FieldValue::U16(443)])
            .expect("flow a2");

        assert_eq!(flow_a1, flow_a2);
        assert_ne!(flow_a1, flow_b);
        assert_eq!(
            index.flow_fields(flow_a1),
            Some(vec![
                OwnedFieldValue::Text("A".into()),
                OwnedFieldValue::U16(443),
            ])
        );
    }

    #[test]
    fn parsed_values_round_trip() {
        let mut index = FlowIndex::new([
            FieldSpec::new("SRC_ADDR", FieldKind::IpAddr),
            FieldSpec::new("PROTOCOL", FieldKind::U8),
            FieldSpec::new("DST_PORT", FieldKind::U16),
            FieldSpec::new("DST_AS", FieldKind::U32),
            FieldSpec::new("BYTES", FieldKind::U64),
            FieldSpec::new("DST_AS_NAME", FieldKind::Text),
        ])
        .expect("index");

        let flow_id = index
            .get_or_insert_parsed_flow(&["1.2.3.4", "6", "443", "13335", "123456", "CLOUDFLARENET"])
            .expect("parsed flow");

        assert_eq!(
            index.flow_fields(flow_id),
            Some(vec![
                OwnedFieldValue::IpAddr("1.2.3.4".parse().expect("ip")),
                OwnedFieldValue::U8(6),
                OwnedFieldValue::U16(443),
                OwnedFieldValue::U32(13335),
                OwnedFieldValue::U64(123456),
                OwnedFieldValue::Text("CLOUDFLARENET".into()),
            ])
        );
    }

    #[test]
    fn ipv6_mapped_ipv4_addresses_reuse_ipv4_field_ids() {
        let mut index =
            FlowIndex::new([FieldSpec::new("SRC_ADDR", FieldKind::IpAddr)]).expect("index");

        let v4 = index
            .get_or_insert_field_value(0, FieldValue::IpAddr(IpAddr::V4(Ipv4Addr::new(1, 2, 3, 4))))
            .expect("v4 field");
        let mapped = index
            .get_or_insert_field_value(
                0,
                FieldValue::IpAddr(IpAddr::V6(
                    "::ffff:1.2.3.4".parse::<Ipv6Addr>().expect("mapped ipv6"),
                )),
            )
            .expect("mapped field");

        assert_eq!(v4, mapped);
        assert_eq!(
            index.field_value(0, mapped),
            Some(FieldValue::IpAddr(IpAddr::V4(Ipv4Addr::new(1, 2, 3, 4))))
        );
    }

    #[test]
    fn find_flow_does_not_insert_missing_values() {
        let mut index = FlowIndex::new([
            FieldSpec::new("SRC_AS_NAME", FieldKind::Text),
            FieldSpec::new("PROTOCOL", FieldKind::Text),
        ])
        .expect("index");

        assert_eq!(
            index
                .find_flow(&[FieldValue::Text("CLOUDFLARE"), FieldValue::Text("TCP")])
                .expect("find flow"),
            None
        );
        assert_eq!(index.flow_count(), 0);

        let flow_id = index
            .get_or_insert_flow(&[FieldValue::Text("CLOUDFLARE"), FieldValue::Text("TCP")])
            .expect("insert flow");

        assert_eq!(
            index
                .find_flow(&[FieldValue::Text("CLOUDFLARE"), FieldValue::Text("TCP")])
                .expect("find inserted flow"),
            Some(flow_id)
        );
    }

    #[test]
    fn find_flow_by_field_ids_reuses_exact_tuple_under_total_hash_collision() {
        let mut index = FlowIndex::with_hasher(
            [
                FieldSpec::new("SRC_AS_NAME", FieldKind::Text),
                FieldSpec::new("DST_PORT", FieldKind::U16),
            ],
            ConstantHashStrategy,
        )
        .expect("index");

        let src_a = index
            .get_or_insert_field_value(0, FieldValue::Text("A"))
            .expect("src a");
        let src_b = index
            .get_or_insert_field_value(0, FieldValue::Text("B"))
            .expect("src b");
        let port_443 = index
            .get_or_insert_field_value(1, FieldValue::U16(443))
            .expect("port 443");

        let flow_a = index
            .insert_flow_by_field_ids(&[src_a, port_443])
            .expect("insert flow a");
        let flow_b = index
            .insert_flow_by_field_ids(&[src_b, port_443])
            .expect("insert flow b");

        assert_eq!(
            index
                .find_flow_by_field_ids(&[src_a, port_443])
                .expect("find a"),
            Some(flow_a)
        );
        assert_eq!(
            index
                .find_flow_by_field_ids(&[src_b, port_443])
                .expect("find b"),
            Some(flow_b)
        );
    }

    #[test]
    fn implicit_defaults_round_trip_sparse_rows() {
        let mut index = FlowIndex::new_with_implicit_defaults([
            FieldSpec::new("FLOW_VERSION", FieldKind::Text),
            FieldSpec::new("PROTOCOL", FieldKind::U8),
            FieldSpec::new("EXPORTER_IP", FieldKind::IpAddr),
            FieldSpec::new("EXPORTER_NAME", FieldKind::Text),
            FieldSpec::new("OUT_IF", FieldKind::U32),
        ])
        .expect("index");

        let flow_id = index
            .get_or_insert_flow(&[
                FieldValue::Text("v5"),
                FieldValue::U8(6),
                FieldValue::IpAddr(IpAddr::V4(Ipv4Addr::UNSPECIFIED)),
                FieldValue::Text(""),
                FieldValue::U32(0),
            ])
            .expect("insert sparse flow");

        assert_eq!(
            index.flow_fields(flow_id),
            Some(vec![
                OwnedFieldValue::Text("v5".into()),
                OwnedFieldValue::U8(6),
                OwnedFieldValue::IpAddr(IpAddr::V4(Ipv4Addr::UNSPECIFIED)),
                OwnedFieldValue::Text("".into()),
                OwnedFieldValue::U32(0),
            ])
        );
        assert_eq!(
            index
                .find_flow(&[
                    FieldValue::Text("v5"),
                    FieldValue::U8(6),
                    FieldValue::IpAddr(IpAddr::V4(Ipv4Addr::UNSPECIFIED)),
                    FieldValue::Text(""),
                    FieldValue::U32(0),
                ])
                .expect("find sparse flow"),
            Some(flow_id)
        );
    }

    #[test]
    fn implicit_defaults_use_less_heap_for_sparse_rollup_rows() {
        let schema = [
            FieldSpec::new("FLOW_VERSION", FieldKind::Text),
            FieldSpec::new("PROTOCOL", FieldKind::U8),
            FieldSpec::new("ETYPE", FieldKind::U16),
            FieldSpec::new("FORWARDING_STATUS", FieldKind::U8),
            FieldSpec::new("EXPORTER_NAME", FieldKind::Text),
            FieldSpec::new("EXPORTER_GROUP", FieldKind::Text),
            FieldSpec::new("EXPORTER_ROLE", FieldKind::Text),
            FieldSpec::new("EXPORTER_SITE", FieldKind::Text),
            FieldSpec::new("EXPORTER_REGION", FieldKind::Text),
            FieldSpec::new("EXPORTER_TENANT", FieldKind::Text),
            FieldSpec::new("IN_IF_NAME", FieldKind::Text),
            FieldSpec::new("OUT_IF_NAME", FieldKind::Text),
            FieldSpec::new("IN_IF_DESCRIPTION", FieldKind::Text),
            FieldSpec::new("OUT_IF_DESCRIPTION", FieldKind::Text),
            FieldSpec::new("SRC_NET_NAME", FieldKind::Text),
            FieldSpec::new("DST_NET_NAME", FieldKind::Text),
            FieldSpec::new("SRC_COUNTRY", FieldKind::Text),
            FieldSpec::new("DST_COUNTRY", FieldKind::Text),
            FieldSpec::new("NEXT_HOP", FieldKind::IpAddr),
            FieldSpec::new("OUT_IF", FieldKind::U32),
        ];
        let mut dense = FlowIndex::new(schema.clone()).expect("dense index");
        let mut sparse = FlowIndex::new_with_implicit_defaults(schema).expect("sparse index");

        for flow_id in 0..10_000u32 {
            let values = [
                FieldValue::Text("v5"),
                FieldValue::U8(6),
                FieldValue::U16(2048),
                FieldValue::U8(0),
                FieldValue::Text("router-a"),
                FieldValue::Text(""),
                FieldValue::Text(""),
                FieldValue::Text(""),
                FieldValue::Text(""),
                FieldValue::Text(""),
                FieldValue::Text(""),
                FieldValue::Text(""),
                FieldValue::Text(""),
                FieldValue::Text(""),
                FieldValue::Text(""),
                FieldValue::Text(""),
                FieldValue::Text("US"),
                FieldValue::Text("DE"),
                FieldValue::IpAddr(IpAddr::V4(Ipv4Addr::UNSPECIFIED)),
                FieldValue::U32(flow_id),
            ];
            dense.get_or_insert_flow(&values).expect("dense flow");
            sparse.get_or_insert_flow(&values).expect("sparse flow");
        }

        assert!(
            sparse.estimated_heap_bytes() + 200_000 < dense.estimated_heap_bytes(),
            "expected sparse defaults to materially reduce row heap: dense={} sparse={}",
            dense.estimated_heap_bytes(),
            sparse.estimated_heap_bytes(),
        );
    }

    proptest! {
        #[test]
        fn collision_heavy_flow_interning_keeps_exact_distinct_tuples(
            tuples in prop::collection::vec(
                (
                    "[A-Z]{1,8}",
                    1u16..65535u16,
                ),
                1..128,
            )
        ) {
            let mut index = FlowIndex::with_hasher(
                [
                    FieldSpec::new("SRC_AS_NAME", FieldKind::Text),
                    FieldSpec::new("DST_PORT", FieldKind::U16),
                ],
                ConstantHashStrategy,
            ).expect("index");

            let mut expected = BTreeSet::new();
            for (name, port) in &tuples {
                expected.insert((name.clone(), *port));
                let _ = index.get_or_insert_flow(&[
                    FieldValue::Text(name),
                    FieldValue::U16(*port),
                ]).expect("flow");
            }

            prop_assert_eq!(index.flow_count(), expected.len());

            let materialized = (0..index.flow_count())
                .map(|flow_id| index.flow_fields(flow_id as u32).expect("flow fields"))
                .map(|fields| match (&fields[0], &fields[1]) {
                    (OwnedFieldValue::Text(name), OwnedFieldValue::U16(port)) => (name.to_string(), *port),
                    other => panic!("unexpected materialized fields: {other:?}"),
                })
                .collect::<BTreeSet<_>>();

            prop_assert_eq!(materialized, expected);
        }
    }
}
