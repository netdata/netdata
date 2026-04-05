use crate::facet_catalog::FacetValueKind;
use bitvec::prelude::*;
use hashbrown::HashTable;
use roaring::RoaringTreemap;
use serde::{Deserialize, Serialize};
use std::hash::Hasher;
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr};
use twox_hash::XxHash64;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub(super) enum PersistedFacetStore {
    Text(Vec<String>),
    DenseU8(Vec<u8>),
    DenseU16(Vec<u16>),
    SparseU32(Vec<u32>),
    SparseU64(Vec<u64>),
    IpAddr(Vec<PersistedPackedIpAddr>),
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize)]
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
            Self::IpAddr(store) => PersistedFacetStore::IpAddr(
                store
                    .values
                    .iter()
                    .map(|value| PersistedPackedIpAddr {
                        family: value.family,
                        bytes: value.bytes,
                    })
                    .collect(),
            ),
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
            bits: bitvec![usize, Lsb0; 0; N],
            len: 0,
        }
    }

    fn insert(&mut self, value: usize) -> bool {
        if value >= N || self.bits[value] {
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
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
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
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(super) struct PackedIpAddr {
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
            IpAddr::V6(ip) => Self {
                family: 6,
                bytes: ip.octets(),
            },
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

#[derive(Debug, Clone, Default)]
pub(super) struct IpValueStore {
    lookup: HashTable<u32>,
    values: Vec<PackedIpAddr>,
}

impl IpValueStore {
    fn new() -> Self {
        Self::default()
    }

    fn len(&self) -> usize {
        self.values.len()
    }

    fn insert(&mut self, value: PackedIpAddr) -> bool {
        let hash = hash_ip(value);
        if self
            .lookup
            .find(hash, |field_id| self.values[*field_id as usize] == value)
            .is_some()
        {
            return false;
        }

        let field_id = self.values.len() as u32;
        self.values.push(value);
        let values = &self.values;
        self.lookup
            .insert_unique(hash, field_id, move |existing_id| {
                hash_ip(values[*existing_id as usize])
            });
        true
    }

    fn collect_strings(&self, limit: Option<usize>) -> Vec<String> {
        let mut values = self
            .values
            .iter()
            .map(|value| value.into_ip().to_string())
            .collect::<Vec<_>>();
        values.sort_unstable();
        if let Some(limit) = limit {
            values.truncate(limit);
        }
        values
    }

    fn prefix_matches(&self, prefix: &str, limit: usize) -> Vec<String> {
        let mut values = Vec::new();
        for value in &self.values {
            let rendered = value.into_ip().to_string();
            if rendered.starts_with(prefix) {
                values.push(rendered);
            }
        }
        values.sort_unstable();
        values.truncate(limit);
        values
    }
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
