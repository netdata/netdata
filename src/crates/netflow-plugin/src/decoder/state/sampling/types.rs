use super::*;
use lru::LruCache;
use std::num::NonZeroUsize;

pub(crate) const DEFAULT_SAMPLING_CACHE_MAX_ENTRIES: usize = 100_000;
pub(crate) const DEFAULT_SAMPLING_CACHE_MAX_ENTRIES_PER_STREAM: usize = 65_536;

#[derive(Debug)]
pub(crate) struct SamplingState {
    pub(crate) values: HashMap<SamplingEntryKey, u64>,
    pub(crate) global_lru: LruCache<SamplingEntryKey, ()>,
    pub(crate) per_stream_lru: HashMap<DecoderStateNamespaceKey, LruCache<u64, ()>>,
    pub(crate) per_stream_capacity: NonZeroUsize,
}

impl Default for SamplingState {
    fn default() -> Self {
        Self::new(
            DEFAULT_SAMPLING_CACHE_MAX_ENTRIES,
            DEFAULT_SAMPLING_CACHE_MAX_ENTRIES_PER_STREAM,
        )
    }
}

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub(crate) struct SamplingEntryKey {
    pub(crate) namespace: DecoderStateNamespaceKey,
    pub(crate) sampler_id: u64,
}
