use crate::facet_catalog::FacetValueKind;
use allocative::{Allocative, Key, Visitor};
use bitvec::prelude::*;
use hashbrown::HashTable;
use roaring::RoaringTreemap;
use serde::{Deserialize, Serialize};
use std::hash::Hasher;
use std::mem::size_of;
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr};
use twox_hash::XxHash64;

#[derive(Debug, Clone, Serialize, Deserialize, allocative::Allocative)]
pub(super) enum PersistedFacetStore {
    Text(Vec<String>),
    DenseU8(Vec<u8>),
    DenseU16(Vec<u16>),
    SparseU32(Vec<u32>),
    SparseU64(Vec<u64>),
    IpAddr(Vec<PersistedPackedIpAddr>),
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize, allocative::Allocative)]
pub(super) struct PersistedPackedIpAddr {
    family: u8,
    bytes: [u8; 16],
}

#[derive(Debug, Clone)]
pub(super) enum FacetStore {
    Text(TextValueStore),
    DenseU8(DenseBitSet<256>),
    DenseU16(DenseBitSet<65_536>),
    SparseU32(RoaringTreemap),
    SparseU64(RoaringTreemap),
    IpAddr(IpValueStore),
}

#[derive(Debug, Clone, Copy)]
pub(super) enum FacetStoreValueRef<'a> {
    Text(&'a str),
    U8(u8),
    U16(u16),
    U32(u32),
    U64(u64),
    IpAddr(PackedIpAddr),
}

impl FacetStoreValueRef<'_> {
    pub(super) fn render(self) -> String {
        match self {
            Self::Text(value) => value.to_string(),
            Self::U8(value) => value.to_string(),
            Self::U16(value) => value.to_string(),
            Self::U32(value) => value.to_string(),
            Self::U64(value) => value.to_string(),
            Self::IpAddr(value) => value.render(),
        }
    }
}

impl FacetStore {
    pub(super) fn new(kind: FacetValueKind) -> Self {
        match kind {
            FacetValueKind::Text => Self::Text(TextValueStore::new()),
            FacetValueKind::DenseU8 => Self::DenseU8(DenseBitSet::new()),
            FacetValueKind::DenseU16 => Self::DenseU16(DenseBitSet::new()),
            FacetValueKind::SparseU32 => Self::SparseU32(RoaringTreemap::new()),
            FacetValueKind::SparseU64 => Self::SparseU64(RoaringTreemap::new()),
            FacetValueKind::IpAddr => Self::IpAddr(IpValueStore::new()),
        }
    }

    pub(super) fn from_persisted(kind: FacetValueKind, persisted: PersistedFacetStore) -> Self {
        match (kind, persisted) {
            (FacetValueKind::Text, PersistedFacetStore::Text(values)) => {
                let mut store = TextValueStore::new();
                for value in values {
                    store.insert(&value);
                }
                Self::Text(store)
            }
            (FacetValueKind::DenseU8, PersistedFacetStore::DenseU8(values)) => {
                let mut store = DenseBitSet::<256>::new();
                for value in values {
                    let _ = store.insert(value as usize);
                }
                Self::DenseU8(store)
            }
            (FacetValueKind::DenseU16, PersistedFacetStore::DenseU16(values)) => {
                let mut store = DenseBitSet::<65_536>::new();
                for value in values {
                    let _ = store.insert(value as usize);
                }
                Self::DenseU16(store)
            }
            (FacetValueKind::SparseU32, PersistedFacetStore::SparseU32(values)) => {
                let mut store = RoaringTreemap::new();
                for value in values {
                    let _ = store.insert(value as u64);
                }
                Self::SparseU32(store)
            }
            (FacetValueKind::SparseU64, PersistedFacetStore::SparseU64(values)) => {
                let mut store = RoaringTreemap::new();
                for value in values {
                    let _ = store.insert(value);
                }
                Self::SparseU64(store)
            }
            (FacetValueKind::IpAddr, PersistedFacetStore::IpAddr(values)) => {
                let mut store = IpValueStore::new();
                for value in values {
                    let _ = store.insert(PackedIpAddr {
                        family: value.family,
                        bytes: value.bytes,
                    });
                }
                Self::IpAddr(store)
            }
            (kind, _) => Self::new(kind),
        }
    }

    pub(super) fn insert_raw(&mut self, raw: &str) -> bool {
        match self {
            Self::Text(store) => store.insert(raw),
            Self::DenseU8(store) => raw
                .parse::<u8>()
                .ok()
                .is_some_and(|value| store.insert(value as usize)),
            Self::DenseU16(store) => raw
                .parse::<u16>()
                .ok()
                .is_some_and(|value| store.insert(value as usize)),
            Self::SparseU32(store) => raw
                .parse::<u32>()
                .ok()
                .is_some_and(|value| store.insert(value as u64)),
            Self::SparseU64(store) => raw
                .parse::<u64>()
                .ok()
                .is_some_and(|value| store.insert(value)),
            Self::IpAddr(store) => raw
                .parse::<IpAddr>()
                .ok()
                .is_some_and(|value| store.insert(PackedIpAddr::from_ip(value))),
        }
    }

    pub(super) fn insert_text(&mut self, value: &str) -> bool {
        match self {
            Self::Text(store) => store.insert(value),
            _ => false,
        }
    }

    pub(super) fn insert_u8(&mut self, value: u8) -> bool {
        match self {
            Self::DenseU8(store) => store.insert(value as usize),
            _ => false,
        }
    }

    pub(super) fn insert_u16(&mut self, value: u16) -> bool {
        match self {
            Self::DenseU16(store) => store.insert(value as usize),
            _ => false,
        }
    }

    pub(super) fn insert_u32(&mut self, value: u32) -> bool {
        match self {
            Self::SparseU32(store) => store.insert(value as u64),
            _ => false,
        }
    }

    pub(super) fn insert_u64(&mut self, value: u64) -> bool {
        match self {
            Self::SparseU64(store) => store.insert(value),
            _ => false,
        }
    }

    pub(super) fn insert_ip_addr(&mut self, value: IpAddr) -> bool {
        match self {
            Self::IpAddr(store) => store.insert(PackedIpAddr::from_ip(value)),
            _ => false,
        }
    }

    pub(super) fn insert_value_ref(&mut self, value: FacetStoreValueRef<'_>) -> bool {
        match (self, value) {
            (Self::Text(store), FacetStoreValueRef::Text(value)) => store.insert(value),
            (Self::DenseU8(store), FacetStoreValueRef::U8(value)) => store.insert(value as usize),
            (Self::DenseU16(store), FacetStoreValueRef::U16(value)) => store.insert(value as usize),
            (Self::SparseU32(store), FacetStoreValueRef::U32(value)) => store.insert(value as u64),
            (Self::SparseU64(store), FacetStoreValueRef::U64(value)) => store.insert(value),
            (Self::IpAddr(store), FacetStoreValueRef::IpAddr(value)) => store.insert(value),
            _ => false,
        }
    }

    pub(super) fn contains_value_ref(&self, value: FacetStoreValueRef<'_>) -> bool {
        match (self, value) {
            (Self::Text(store), FacetStoreValueRef::Text(value)) => store.contains(value),
            (Self::DenseU8(store), FacetStoreValueRef::U8(value)) => store.contains(value as usize),
            (Self::DenseU16(store), FacetStoreValueRef::U16(value)) => {
                store.contains(value as usize)
            }
            (Self::SparseU32(store), FacetStoreValueRef::U32(value)) => {
                store.contains(value as u64)
            }
            (Self::SparseU64(store), FacetStoreValueRef::U64(value)) => store.contains(value),
            (Self::IpAddr(store), FacetStoreValueRef::IpAddr(value)) => store.contains(value),
            _ => false,
        }
    }

    pub(super) fn len(&self) -> usize {
        match self {
            Self::Text(store) => store.len(),
            Self::DenseU8(store) => store.len(),
            Self::DenseU16(store) => store.len(),
            Self::SparseU32(store) => store.len() as usize,
            Self::SparseU64(store) => store.len() as usize,
            Self::IpAddr(store) => store.len(),
        }
    }

    pub(super) fn collect_strings(&self, limit: Option<usize>) -> Vec<String> {
        match self {
            Self::Text(store) => store.collect_strings(limit),
            Self::DenseU8(store) => store.collect_strings(limit),
            Self::DenseU16(store) => store.collect_strings(limit),
            Self::SparseU32(store) => collect_roaring_strings(store, limit),
            Self::SparseU64(store) => collect_roaring_strings(store, limit),
            Self::IpAddr(store) => store.collect_strings(limit),
        }
    }

    pub(super) fn visit_values<'a>(&'a self, mut visitor: impl FnMut(FacetStoreValueRef<'a>)) {
        match self {
            Self::Text(store) => {
                store.visit_values(|value| visitor(FacetStoreValueRef::Text(value)))
            }
            Self::DenseU8(store) => {
                for value in store.iter_indices() {
                    visitor(FacetStoreValueRef::U8(value as u8));
                }
            }
            Self::DenseU16(store) => {
                for value in store.iter_indices() {
                    visitor(FacetStoreValueRef::U16(value as u16));
                }
            }
            Self::SparseU32(store) => {
                for value in store.iter() {
                    visitor(FacetStoreValueRef::U32(value as u32));
                }
            }
            Self::SparseU64(store) => {
                for value in store.iter() {
                    visitor(FacetStoreValueRef::U64(value));
                }
            }
            Self::IpAddr(store) => {
                store.visit_values(|value| visitor(FacetStoreValueRef::IpAddr(value)))
            }
        }
    }

    pub(super) fn prefix_matches(&self, prefix: &str, limit: usize) -> Vec<String> {
        match self {
            Self::Text(store) => store.prefix_matches(prefix, limit),
            Self::DenseU8(store) => store.prefix_matches(prefix, limit),
            Self::DenseU16(store) => store.prefix_matches(prefix, limit),
            Self::SparseU32(store) => prefix_match_roaring(store, prefix, limit),
            Self::SparseU64(store) => prefix_match_roaring(store, prefix, limit),
            Self::IpAddr(store) => store.prefix_matches(prefix, limit),
        }
    }

    pub(super) fn persist(&self) -> PersistedFacetStore {
        match self {
            Self::Text(store) => PersistedFacetStore::Text(store.collect_strings(None)),
            Self::DenseU8(store) => PersistedFacetStore::DenseU8(
                store.iter_indices().map(|value| value as u8).collect(),
            ),
            Self::DenseU16(store) => PersistedFacetStore::DenseU16(
                store.iter_indices().map(|value| value as u16).collect(),
            ),
            Self::SparseU32(store) => {
                PersistedFacetStore::SparseU32(store.iter().map(|value| value as u32).collect())
            }
            Self::SparseU64(store) => PersistedFacetStore::SparseU64(store.iter().collect()),
            Self::IpAddr(store) => PersistedFacetStore::IpAddr(store.persisted_values()),
        }
    }

    pub(super) fn merge_from(&mut self, other: &FacetStore) -> bool {
        match (self, other) {
            (Self::Text(dst), Self::Text(src)) => dst.merge_from(src),
            (Self::DenseU8(dst), Self::DenseU8(src)) => dst.merge_from(src),
            (Self::DenseU16(dst), Self::DenseU16(src)) => dst.merge_from(src),
            (Self::SparseU32(dst), Self::SparseU32(src)) => {
                let mut changed = false;
                for value in src.iter() {
                    changed |= dst.insert(value);
                }
                changed
            }
            (Self::SparseU64(dst), Self::SparseU64(src)) => {
                let mut changed = false;
                for value in src.iter() {
                    changed |= dst.insert(value);
                }
                changed
            }
            (Self::IpAddr(dst), Self::IpAddr(src)) => dst.merge_from(src),
            _ => false,
        }
    }

    pub(super) fn estimated_heap_bytes(&self) -> usize {
        match self {
            Self::Text(store) => store.estimated_heap_bytes(),
            Self::DenseU8(store) => store.estimated_heap_bytes(),
            Self::DenseU16(store) => store.estimated_heap_bytes(),
            Self::SparseU32(store) | Self::SparseU64(store) => store.serialized_size(),
            Self::IpAddr(store) => store.estimated_heap_bytes(),
        }
    }
}

#[derive(Debug, Clone)]
pub(super) struct DenseBitSet<const N: usize> {
    bits: BitVec<usize, Lsb0>,
    len: usize,
}

impl<const N: usize> DenseBitSet<N> {
    fn new() -> Self {
        Self {
            bits: BitVec::new(),
            len: 0,
        }
    }

    fn insert(&mut self, value: usize) -> bool {
        if value >= N {
            return false;
        }
        if self.bits.is_empty() {
            self.bits.resize(N, false);
        }
        if self.bits[value] {
            return false;
        }
        self.bits.set(value, true);
        self.len += 1;
        true
    }

    fn len(&self) -> usize {
        self.len
    }

    fn iter_indices(&self) -> impl Iterator<Item = usize> + '_ {
        self.bits.iter_ones()
    }

    fn contains(&self, value: usize) -> bool {
        value < self.bits.len() && self.bits[value]
    }

    fn collect_strings(&self, limit: Option<usize>) -> Vec<String> {
        let mut values = Vec::new();
        for value in self.iter_indices() {
            values.push(value.to_string());
            if let Some(limit) = limit
                && values.len() >= limit
            {
                break;
            }
        }
        values
    }

    fn prefix_matches(&self, prefix: &str, limit: usize) -> Vec<String> {
        let mut values = Vec::new();
        for value in self.iter_indices() {
            let rendered = value.to_string();
            if rendered.starts_with(prefix) {
                values.push(rendered);
                if values.len() >= limit {
                    break;
                }
            }
        }
        values
    }

    fn merge_from(&mut self, other: &Self) -> bool {
        let mut changed = false;
        for value in other.iter_indices() {
            changed |= self.insert(value);
        }
        changed
    }

    fn estimated_heap_bytes(&self) -> usize {
        self.bits.as_raw_slice().len() * size_of::<usize>()
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize, allocative::Allocative)]
struct TextEntry {
    offset: u32,
    len: u32,
}

#[derive(Debug, Clone, Default)]
pub(super) struct TextValueStore {
    lookup: HashTable<u32>,
    entries: Vec<TextEntry>,
    arena: Vec<u8>,
}

impl TextValueStore {
    fn new() -> Self {
        Self::default()
    }

    fn len(&self) -> usize {
        self.entries.len()
    }

    fn insert(&mut self, value: &str) -> bool {
        let hash = hash_bytes(value.as_bytes());
        if self
            .lookup
            .find(hash, |field_id| self.value_str(*field_id) == Some(value))
            .is_some()
        {
            return false;
        }

        let field_id = self.entries.len() as u32;
        let offset = self.arena.len() as u32;
        let len = value.len() as u32;

        self.arena.extend_from_slice(value.as_bytes());
        self.entries.push(TextEntry { offset, len });

        let entries = &self.entries;
        let arena = &self.arena;
        self.lookup
            .insert_unique(hash, field_id, move |existing_id| {
                let entry = entries[*existing_id as usize];
                let start = entry.offset as usize;
                let end = start + entry.len as usize;
                hash_bytes(&arena[start..end])
            });

        true
    }

    fn contains(&self, value: &str) -> bool {
        let hash = hash_bytes(value.as_bytes());
        self.lookup
            .find(hash, |field_id| self.value_str(*field_id) == Some(value))
            .is_some()
    }

    fn value_str(&self, field_id: u32) -> Option<&str> {
        let entry = self.entries.get(field_id as usize)?;
        let start = entry.offset as usize;
        let end = start + entry.len as usize;
        std::str::from_utf8(self.arena.get(start..end)?).ok()
    }

    fn collect_strings(&self, limit: Option<usize>) -> Vec<String> {
        let mut values = (0..self.entries.len())
            .filter_map(|field_id| self.value_str(field_id as u32).map(str::to_string))
            .collect::<Vec<_>>();
        values.sort_unstable();
        if let Some(limit) = limit {
            values.truncate(limit);
        }
        values
    }

    fn visit_values<'a>(&'a self, mut visitor: impl FnMut(&'a str)) {
        for field_id in 0..self.entries.len() {
            let Some(value) = self.value_str(field_id as u32) else {
                continue;
            };
            visitor(value);
        }
    }

    fn prefix_matches(&self, prefix: &str, limit: usize) -> Vec<String> {
        let mut values = Vec::new();
        for field_id in 0..self.entries.len() {
            let Some(value) = self.value_str(field_id as u32) else {
                continue;
            };
            if value.starts_with(prefix) {
                values.push(value.to_string());
            }
        }
        values.sort_unstable();
        values.truncate(limit);
        values
    }

    fn merge_from(&mut self, other: &Self) -> bool {
        let mut changed = false;
        for field_id in 0..other.entries.len() {
            let Some(value) = other.value_str(field_id as u32) else {
                continue;
            };
            changed |= self.insert(value);
        }
        changed
    }

    fn estimated_heap_bytes(&self) -> usize {
        self.lookup.allocation_size()
            + self.entries.capacity() * size_of::<TextEntry>()
            + self.arena.capacity()
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, allocative::Allocative)]
pub(super) struct PackedIpAddr {
    family: u8,
    bytes: [u8; 16],
}

impl PackedIpAddr {
    pub(super) fn from_ip(value: IpAddr) -> Self {
        match value {
            IpAddr::V4(ip) => {
                let mut bytes = [0u8; 16];
                bytes[..4].copy_from_slice(&ip.octets());
                Self { family: 4, bytes }
            }
            IpAddr::V6(ip) => Self {
                family: 6,
                bytes: ip.octets(),
            },
        }
    }

    fn to_ip(self) -> IpAddr {
        match self.family {
            4 => {
                let mut octets = [0u8; 4];
                octets.copy_from_slice(&self.bytes[..4]);
                IpAddr::V4(Ipv4Addr::from(octets))
            }
            _ => IpAddr::V6(Ipv6Addr::from(self.bytes)),
        }
    }

    fn render(self) -> String {
        self.to_ip().to_string()
    }
}

#[derive(Debug, Clone, Default)]
pub(super) struct IpValueStore {
    v4_values: RoaringTreemap,
    v6_lookup: HashTable<u32>,
    v6_values: Vec<[u8; 16]>,
}

impl IpValueStore {
    fn new() -> Self {
        Self::default()
    }

    fn len(&self) -> usize {
        self.v4_values.len() as usize + self.v6_values.len()
    }

    fn insert(&mut self, value: PackedIpAddr) -> bool {
        if value.family == 4 {
            return self.v4_values.insert(packed_ipv4_bits(value) as u64);
        }

        let hash = hash_ip(value);
        if self
            .v6_lookup
            .find(hash, |field_id| {
                self.v6_values[*field_id as usize] == value.bytes
            })
            .is_some()
        {
            return false;
        }

        let field_id = self.v6_values.len() as u32;
        self.v6_values.push(value.bytes);
        let values = &self.v6_values;
        self.v6_lookup
            .insert_unique(hash, field_id, move |existing_id| {
                hash_ip(PackedIpAddr {
                    family: 6,
                    bytes: values[*existing_id as usize],
                })
            });
        true
    }

    fn contains(&self, value: PackedIpAddr) -> bool {
        if value.family == 4 {
            return self.v4_values.contains(packed_ipv4_bits(value) as u64);
        }
        let hash = hash_ip(value);
        self.v6_lookup
            .find(hash, |field_id| {
                self.v6_values[*field_id as usize] == value.bytes
            })
            .is_some()
    }

    fn collect_strings(&self, limit: Option<usize>) -> Vec<String> {
        let mut values = self
            .v4_values
            .iter()
            .map(|value| IpAddr::V4(Ipv4Addr::from((value as u32).to_be_bytes())).to_string())
            .collect::<Vec<_>>();
        values.extend(
            self.v6_values
                .iter()
                .map(|value| IpAddr::V6(Ipv6Addr::from(*value)).to_string()),
        );
        values.sort_unstable();
        if let Some(limit) = limit {
            values.truncate(limit);
        }
        values
    }

    fn prefix_matches(&self, prefix: &str, limit: usize) -> Vec<String> {
        let mut values = Vec::new();
        for value in self.v4_values.iter() {
            let rendered = IpAddr::V4(Ipv4Addr::from((value as u32).to_be_bytes())).to_string();
            if rendered.starts_with(prefix) {
                values.push(rendered);
            }
        }
        for value in &self.v6_values {
            let rendered = IpAddr::V6(Ipv6Addr::from(*value)).to_string();
            if rendered.starts_with(prefix) {
                values.push(rendered);
            }
        }
        values.sort_unstable();
        values.truncate(limit);
        values
    }

    fn merge_from(&mut self, other: &Self) -> bool {
        let mut changed = false;
        for value in other.v4_values.iter() {
            changed |= self.v4_values.insert(value);
        }
        for value in &other.v6_values {
            changed |= self.insert(PackedIpAddr {
                family: 6,
                bytes: *value,
            });
        }
        changed
    }

    fn estimated_heap_bytes(&self) -> usize {
        self.v4_values.serialized_size()
            + self.v6_lookup.allocation_size()
            + self.v6_values.capacity() * size_of::<[u8; 16]>()
    }

    fn persisted_values(&self) -> Vec<PersistedPackedIpAddr> {
        let mut values = self
            .v4_values
            .iter()
            .map(|value| {
                let mut bytes = [0u8; 16];
                bytes[..4].copy_from_slice(&(value as u32).to_be_bytes());
                PersistedPackedIpAddr { family: 4, bytes }
            })
            .collect::<Vec<_>>();
        values.extend(self.v6_values.iter().map(|value| PersistedPackedIpAddr {
            family: 6,
            bytes: *value,
        }));
        values
    }

    fn visit_values(&self, mut visitor: impl FnMut(PackedIpAddr)) {
        for value in self.v4_values.iter() {
            visitor(PackedIpAddr {
                family: 4,
                bytes: ipv4_treemap_bytes(value as u32),
            });
        }
        for value in &self.v6_values {
            visitor(PackedIpAddr {
                family: 6,
                bytes: *value,
            });
        }
    }
}

fn ipv4_treemap_bytes(value: u32) -> [u8; 16] {
    let mut bytes = [0u8; 16];
    bytes[..4].copy_from_slice(&value.to_be_bytes());
    bytes
}

fn packed_ipv4_bits(value: PackedIpAddr) -> u32 {
    u32::from_be_bytes([
        value.bytes[0],
        value.bytes[1],
        value.bytes[2],
        value.bytes[3],
    ])
}

fn collect_roaring_strings(bitmap: &RoaringTreemap, limit: Option<usize>) -> Vec<String> {
    let mut values = Vec::new();
    for value in bitmap.iter() {
        values.push(value.to_string());
        if let Some(limit) = limit
            && values.len() >= limit
        {
            break;
        }
    }
    values
}

fn prefix_match_roaring(bitmap: &RoaringTreemap, prefix: &str, limit: usize) -> Vec<String> {
    let mut values = Vec::new();
    for value in bitmap.iter() {
        let rendered = value.to_string();
        if rendered.starts_with(prefix) {
            values.push(rendered);
            if values.len() >= limit {
                break;
            }
        }
    }
    values
}

fn hash_bytes(bytes: &[u8]) -> u64 {
    let mut hasher = XxHash64::default();
    hasher.write(bytes);
    hasher.finish()
}

fn hash_ip(value: PackedIpAddr) -> u64 {
    let mut hasher = XxHash64::default();
    hasher.write_u8(value.family);
    hasher.write(&value.bytes);
    hasher.finish()
}

impl Allocative for FacetStore {
    fn visit<'a, 'b: 'a>(&self, visitor: &'a mut Visitor<'b>) {
        let mut visitor = visitor.enter_self(self);
        match self {
            Self::Text(store) => visitor.visit_field(Key::new("text"), store),
            Self::DenseU8(store) => visitor.visit_field(Key::new("dense_u8"), store),
            Self::DenseU16(store) => visitor.visit_field(Key::new("dense_u16"), store),
            Self::SparseU32(store) => visitor.visit_field_with(
                Key::new("sparse_u32"),
                size_of::<RoaringTreemap>(),
                |visitor| visit_roaring_treemap(store, visitor),
            ),
            Self::SparseU64(store) => visitor.visit_field_with(
                Key::new("sparse_u64"),
                size_of::<RoaringTreemap>(),
                |visitor| visit_roaring_treemap(store, visitor),
            ),
            Self::IpAddr(store) => visitor.visit_field(Key::new("ip_addr"), store),
        }
        visitor.exit();
    }
}

impl<const N: usize> Allocative for DenseBitSet<N> {
    fn visit<'a, 'b: 'a>(&self, visitor: &'a mut Visitor<'b>) {
        let mut visitor = visitor.enter_self_sized::<Self>();
        visitor.visit_simple(Key::new("bits"), dense_bitset_capacity_bytes(self));
        visitor.visit_simple(
            Key::new("unused_bits"),
            dense_bitset_unused_capacity_bytes(self),
        );
        visitor.exit();
    }
}

impl Allocative for TextValueStore {
    fn visit<'a, 'b: 'a>(&self, visitor: &'a mut Visitor<'b>) {
        let mut visitor = visitor.enter_self_sized::<Self>();
        visitor.visit_simple(Key::new("lookup"), self.lookup.allocation_size());
        visitor.visit_field(Key::new("entries"), &self.entries);
        visitor.visit_field(Key::new("arena"), &self.arena);
        visitor.exit();
    }
}

impl Allocative for IpValueStore {
    fn visit<'a, 'b: 'a>(&self, visitor: &'a mut Visitor<'b>) {
        let mut visitor = visitor.enter_self_sized::<Self>();
        visitor.visit_field_with(
            Key::new("v4_values"),
            size_of::<RoaringTreemap>(),
            |visitor| {
                visit_roaring_treemap(&self.v4_values, visitor);
            },
        );
        visitor.visit_simple(Key::new("v6_lookup"), self.v6_lookup.allocation_size());
        visitor.visit_field(Key::new("v6_values"), &self.v6_values);
        visitor.exit();
    }
}

fn dense_bitset_capacity_bytes<const N: usize>(store: &DenseBitSet<N>) -> usize {
    store
        .bits
        .capacity()
        .div_ceil(usize::BITS as usize)
        .saturating_mul(size_of::<usize>())
}

fn dense_bitset_unused_capacity_bytes<const N: usize>(store: &DenseBitSet<N>) -> usize {
    store
        .bits
        .capacity()
        .saturating_sub(store.bits.len())
        .div_ceil(usize::BITS as usize)
        .saturating_mul(size_of::<usize>())
}

fn visit_roaring_treemap<'a, 'b: 'a>(store: &RoaringTreemap, visitor: &'a mut Visitor<'b>) {
    let mut visitor = visitor.enter_self(store);
    for (partition, bitmap) in store.bitmaps() {
        visitor.visit_field(Key::new("partition"), &partition);
        visitor.visit_field(Key::new("bitmap"), bitmap);
    }
    visitor.exit();
}
