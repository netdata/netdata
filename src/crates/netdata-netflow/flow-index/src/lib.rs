use std::collections::BTreeSet;
use std::hash::Hasher;
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr};

use hashbrown::HashTable;
use thiserror::Error;
use twox_hash::XxHash64;

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
    flow_values: Vec<u32>,
    hasher: H,
}

impl FlowIndex<XxHash64Strategy> {
    pub fn new(fields: impl IntoIterator<Item = FieldSpec>) -> Result<Self, FlowIndexError> {
        Self::with_hasher(fields, XxHash64Strategy)
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
        let schema = FlowSchema::new(fields)?;
        let field_stores = schema
            .fields()
            .iter()
            .map(|field| FieldStore::new(field.kind()))
            .collect::<Vec<_>>()
            .into_boxed_slice();

        Ok(Self {
            schema,
            field_stores,
            flow_lookup: HashTable::new(),
            flow_values: Vec::new(),
            hasher,
        })
    }

    pub fn schema(&self) -> &FlowSchema {
        &self.schema
    }

    pub fn flow_count(&self) -> usize {
        if self.schema.is_empty() {
            0
        } else {
            self.flow_values.len() / self.schema.len()
        }
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

        if let Some(existing_id) = self.find_flow_by_field_ids(&field_ids)? {
            return Ok(existing_id);
        }

        self.insert_flow_by_field_ids(&field_ids)
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
        if field_ids.len() != self.schema.len() {
            return Err(FlowIndexError::FieldCountMismatch {
                expected: self.schema.len(),
                actual: field_ids.len(),
            });
        }

        let hash = self.hasher.hash_u32_slice(field_ids);
        Ok(self
            .flow_lookup
            .find(hash, |flow_id| {
                self.flow_slice(*flow_id)
                    .is_some_and(|ids| ids == field_ids)
            })
            .copied())
    }

    pub fn insert_flow_by_field_ids(
        &mut self,
        field_ids: &[FieldId],
    ) -> Result<FlowId, FlowIndexError> {
        if field_ids.len() != self.schema.len() {
            return Err(FlowIndexError::FieldCountMismatch {
                expected: self.schema.len(),
                actual: field_ids.len(),
            });
        }

        let hash = self.hasher.hash_u32_slice(field_ids);
        let flow_id = next_flow_id(self.flow_count())?;
        self.flow_values.extend_from_slice(field_ids);

        let flow_values = &self.flow_values;
        let arity = self.schema.len();
        let hasher = self.hasher.clone();
        self.flow_lookup
            .insert_unique(hash, flow_id, move |existing_id| {
                let start = *existing_id as usize * arity;
                hasher.hash_u32_slice(&flow_values[start..start + arity])
            });

        Ok(flow_id)
    }

    pub fn flow_field_ids(&self, flow_id: FlowId) -> Option<&[FieldId]> {
        self.flow_slice(flow_id)
    }

    pub fn flow_fields(&self, flow_id: FlowId) -> Option<Vec<OwnedFieldValue>> {
        let ids = self.flow_slice(flow_id)?;
        let mut values = Vec::with_capacity(ids.len());
        for (index, field_id) in ids.iter().copied().enumerate() {
            values.push(self.field_stores[index].owned_value(field_id)?);
        }
        Some(values)
    }

    fn flow_slice(&self, flow_id: FlowId) -> Option<&[u32]> {
        let arity = self.schema.len();
        let start = flow_id as usize * arity;
        let end = start + arity;
        self.flow_values.get(start..end)
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

#[derive(Copy, Clone, Debug, PartialEq, Eq)]
struct PackedIpAddr {
    family: u8,
    bytes: [u8; 16],
}

impl PackedIpAddr {
    fn from_ip(value: IpAddr) -> Self {
        match value {
            IpAddr::V4(ip) => {
                let mut bytes = [0u8; 16];
                bytes[..4].copy_from_slice(&ip.octets());
                Self { family: 4, bytes }
            }
            IpAddr::V6(ip) => ip
                .to_ipv4_mapped()
                .map(IpAddr::V4)
                .map(Self::from_ip)
                .unwrap_or_else(|| Self {
                    family: 6,
                    bytes: ip.octets(),
                }),
        }
    }

    fn into_ip(self) -> IpAddr {
        match self.family {
            4 => {
                let mut octets = [0u8; 4];
                octets.copy_from_slice(&self.bytes[..4]);
                IpAddr::V4(Ipv4Addr::from(octets))
            }
            _ => IpAddr::V6(Ipv6Addr::from(self.bytes)),
        }
    }
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
        }
    };
}

define_numeric_store!(U8FieldStore, u8, U8, hash_u8);
define_numeric_store!(U16FieldStore, u16, U16, hash_u16);
define_numeric_store!(U32FieldStore, u32, U32, hash_u32);
define_numeric_store!(U64FieldStore, u64, U64, hash_u64);

struct IpFieldStore {
    lookup: HashTable<u32>,
    values: Vec<PackedIpAddr>,
}

impl IpFieldStore {
    fn new() -> Self {
        Self {
            lookup: HashTable::new(),
            values: Vec::new(),
        }
    }

    fn get_or_insert<H: HashStrategy>(
        &mut self,
        value: IpAddr,
        hasher: &H,
    ) -> Result<FieldId, FlowIndexError> {
        let packed = PackedIpAddr::from_ip(value);
        let hash = hasher.hash_ip_parts(packed.family, &packed.bytes);
        if let Some(existing_id) = self
            .lookup
            .find(hash, |field_id| self.values[*field_id as usize] == packed)
            .copied()
        {
            return Ok(existing_id);
        }

        let field_id = next_field_id(self.values.len())?;
        self.values.push(packed);

        let values = &self.values;
        let hasher = hasher.clone();
        self.lookup
            .insert_unique(hash, field_id, move |existing_id| {
                let value = values[*existing_id as usize];
                hasher.hash_ip_parts(value.family, &value.bytes)
            });

        Ok(field_id)
    }

    fn find<H: HashStrategy>(&self, value: IpAddr, hasher: &H) -> Option<FieldId> {
        let packed = PackedIpAddr::from_ip(value);
        let hash = hasher.hash_ip_parts(packed.family, &packed.bytes);
        self.lookup
            .find(hash, |field_id| self.values[*field_id as usize] == packed)
            .copied()
    }

    fn value(&self, field_id: FieldId) -> Option<FieldValue<'_>> {
        Some(FieldValue::IpAddr(
            self.values.get(field_id as usize)?.to_owned().into_ip(),
        ))
    }

    fn owned_value(&self, field_id: FieldId) -> Option<OwnedFieldValue> {
        Some(OwnedFieldValue::IpAddr(
            self.values.get(field_id as usize)?.to_owned().into_ip(),
        ))
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
        let mut index = FlowIndex::new([FieldSpec::new("SRC_ADDR", FieldKind::IpAddr)])
            .expect("index");

        let v4 = index
            .get_or_insert_field_value(
                0,
                FieldValue::IpAddr(IpAddr::V4(Ipv4Addr::new(1, 2, 3, 4))),
            )
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
