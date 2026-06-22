//! Generic associative index backed by a finite state transducer.
//!
//! An [`FstIndex<T>`] maps byte-string keys to associated values of type `T`,
//! using an FST for compact key storage and a parallel `Vec<T>` for the values.
//!
//! The FST stores keys in sorted order and maps each key to its position in the
//! values vector. This gives O(key_length) exact lookups and efficient prefix
//! searches, with significantly less memory overhead than a `HashMap` for the
//! keys.
//!
//! # Example
//!
//! ```
//! use fst_index::FstIndex;
//!
//! let index: FstIndex<u32> = FstIndex::build([
//!     ("apple", 1u32),
//!     ("banana", 2),
//!     ("application", 3),
//! ]).unwrap();
//!
//! assert_eq!(index.get(b"banana"), Some(&2));
//! assert_eq!(index.get(b"cherry"), None);
//!
//! // Prefix search: all keys starting with "app"
//! let values = index.prefix_values(b"app");
//! assert_eq!(values.len(), 2); // "apple" and "application"
//! ```

use fst::automaton::Automaton;
use fst::{IntoStreamer, Streamer};

#[cfg(feature = "serde")]
mod serde_impl {
    use super::FstIndex;
    use serde::{Deserialize, Deserializer, Serialize, Serializer};

    #[derive(Serialize, Deserialize)]
    struct FstIndexHelper<T> {
        fst_bytes: Vec<u8>,
        values: Vec<T>,
    }

    impl<T: Serialize + Clone> Serialize for FstIndex<T> {
        fn serialize<S: Serializer>(&self, serializer: S) -> Result<S::Ok, S::Error> {
            let helper = FstIndexHelper {
                fst_bytes: self.map.as_fst().as_bytes().to_vec(),
                values: self.values.clone(),
            };
            helper.serialize(serializer)
        }
    }

    impl<'de, T: Deserialize<'de>> Deserialize<'de> for FstIndex<T> {
        fn deserialize<D: Deserializer<'de>>(deserializer: D) -> Result<Self, D::Error> {
            let helper = FstIndexHelper::<T>::deserialize(deserializer)?;
            let map = fst::Map::new(helper.fst_bytes).map_err(serde::de::Error::custom)?;
            Ok(FstIndex {
                map,
                values: helper.values,
            })
        }
    }
}

/// Error type for FST index construction.
#[derive(Debug)]
pub struct BuildError(String);

impl std::fmt::Display for BuildError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "FST build error: {}", self.0)
    }
}

impl std::error::Error for BuildError {}

impl From<fst::Error> for BuildError {
    fn from(e: fst::Error) -> Self {
        BuildError(e.to_string())
    }
}

/// A generic associative index backed by a finite state transducer.
///
/// Maps byte-string keys to values of type `T`. Keys are stored in a
/// compressed FST; values are stored in a `Vec<T>` ordered by sorted key.
///
/// Supports exact lookup by key and prefix search. Immutable after
/// construction.
pub struct FstIndex<T> {
    map: fst::Map<Vec<u8>>,
    values: Vec<T>,
}

impl<T: std::fmt::Debug> std::fmt::Debug for FstIndex<T> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("FstIndex")
            .field("len", &self.map.len())
            .field("fst_bytes", &self.fst_bytes())
            .finish()
    }
}

impl<T: Clone> Clone for FstIndex<T> {
    fn clone(&self) -> Self {
        let bytes = self.map.as_fst().as_bytes().to_vec();
        Self {
            map: fst::Map::new(bytes).expect("cloning valid FST"),
            values: self.values.clone(),
        }
    }
}

impl<T> FstIndex<T> {
    /// Build an index from an unsorted iterator of `(key, value)` pairs.
    ///
    /// Keys are sorted internally before FST construction. Duplicate keys
    /// are an error.
    pub fn build<K: Ord + AsRef<[u8]>>(
        iter: impl IntoIterator<Item = (K, T)>,
    ) -> Result<Self, BuildError> {
        let mut pairs: Vec<(K, T)> = iter.into_iter().collect();
        pairs.sort_by(|(a, _), (b, _)| a.cmp(b));

        let bump = bumpalo::Bump::with_capacity(32 * 1024 * 1024);
        let mut builder = fst::MapBuilder::memory(&bump);
        let mut values = Vec::with_capacity(pairs.len());

        for (idx, (key, value)) in pairs.into_iter().enumerate() {
            builder
                .insert(key.as_ref(), idx as u64)
                .map_err(|e| BuildError(e.to_string()))?;
            values.push(value);
        }

        let bytes = builder
            .into_inner()
            .map_err(|e| BuildError(e.to_string()))?;
        let map = fst::Map::new(bytes)?;

        Ok(Self { map, values })
    }

    /// Build an index from a pre-sorted iterator of `(key, value)` pairs.
    ///
    /// The caller must ensure keys are in lexicographic order. This avoids
    /// the internal sort performed by [`build`](Self::build).
    pub fn build_from_sorted<K: AsRef<[u8]>>(
        iter: impl IntoIterator<Item = (K, T)>,
    ) -> Result<Self, BuildError> {
        let bump = bumpalo::Bump::with_capacity(32 * 1024 * 1024);
        let mut builder = fst::MapBuilder::memory(&bump);
        let mut values = Vec::new();

        for (idx, (key, value)) in iter.into_iter().enumerate() {
            builder
                .insert(key.as_ref(), idx as u64)
                .map_err(|e| BuildError(e.to_string()))?;
            values.push(value);
        }

        let bytes = builder
            .into_inner()
            .map_err(|e| BuildError(e.to_string()))?;
        let map = fst::Map::new(bytes)?;

        Ok(Self { map, values })
    }

    /// Number of entries in the index.
    pub fn len(&self) -> usize {
        self.map.len()
    }

    /// Returns `true` if the index contains no entries.
    pub fn is_empty(&self) -> bool {
        self.map.len() == 0
    }

    /// Size of the serialized FST data in bytes (keys only, not values).
    pub fn fst_bytes(&self) -> usize {
        self.map.as_fst().as_bytes().len()
    }

    /// Raw FST bytes (the serialized automaton, without the values vector).
    pub fn fst_raw_bytes(&self) -> &[u8] {
        self.map.as_fst().as_bytes()
    }

    /// Look up the value associated with an exact key.
    pub fn get(&self, key: &[u8]) -> Option<&T> {
        self.map.get(key).map(|idx| &self.values[idx as usize])
    }

    /// Look up the value associated with an exact key, returning a mutable
    /// reference.
    pub fn get_mut(&mut self, key: &[u8]) -> Option<&mut T> {
        self.map.get(key).map(|idx| &mut self.values[idx as usize])
    }

    /// Returns `true` if the index contains the given key.
    pub fn contains_key(&self, key: &[u8]) -> bool {
        self.map.contains_key(key)
    }

    /// Collect all values whose keys start with `prefix`.
    ///
    /// Values are returned in key-sorted order.
    pub fn prefix_values(&self, prefix: &[u8]) -> Vec<&T> {
        let automaton = Prefix(prefix);
        let mut stream = self.map.search(&automaton).into_stream();
        let mut result = Vec::new();
        while let Some((_, idx)) = stream.next() {
            result.push(&self.values[idx as usize]);
        }
        result
    }

    /// Collect all `(key, value)` pairs whose keys start with `prefix`.
    ///
    /// Pairs are returned in key-sorted order. Keys are borrowed from the
    /// FST traversal and copied into owned `Vec<u8>` because the FST stream
    /// does not allow borrowing keys across iterations.
    pub fn prefix_pairs(&self, prefix: &[u8]) -> Vec<(Vec<u8>, &T)> {
        let automaton = Prefix(prefix);
        let mut stream = self.map.search(&automaton).into_stream();
        let mut result = Vec::new();
        while let Some((key, idx)) = stream.next() {
            result.push((key.to_vec(), &self.values[idx as usize]));
        }
        result
    }

    /// Call `f` for each value whose key starts with `prefix`.
    ///
    /// This avoids allocating a temporary `Vec` when you only need to process
    /// each value once. Values are visited in key-sorted order.
    pub fn prefix_for_each(&self, prefix: &[u8], mut f: impl FnMut(&[u8], &T)) {
        let automaton = Prefix(prefix);
        let mut stream = self.map.search(&automaton).into_stream();
        while let Some((key, idx)) = stream.next() {
            f(key, &self.values[idx as usize]);
        }
    }

    /// Call `f` for each `(key, value)` pair in the index.
    ///
    /// Equivalent to `prefix_for_each(b"", f)` but with clearer intent.
    /// Values are visited in key-sorted order.
    pub fn for_each(&self, f: impl FnMut(&[u8], &T)) {
        self.prefix_for_each(b"", f);
    }

    /// Get a slice of all values, ordered by their sorted key.
    pub fn values(&self) -> &[T] {
        &self.values
    }
}

/// Automaton that matches keys starting with a given byte prefix.
struct Prefix<'a>(&'a [u8]);

impl Automaton for Prefix<'_> {
    type State = Option<usize>;

    #[inline]
    fn start(&self) -> Option<usize> {
        Some(0)
    }

    #[inline]
    fn is_match(&self, state: &Option<usize>) -> bool {
        matches!(state, Some(pos) if *pos >= self.0.len())
    }

    #[inline]
    fn can_match(&self, state: &Option<usize>) -> bool {
        state.is_some()
    }

    #[inline]
    fn will_always_match(&self, state: &Option<usize>) -> bool {
        matches!(state, Some(pos) if *pos >= self.0.len())
    }

    #[inline]
    fn accept(&self, state: &Option<usize>, byte: u8) -> Option<usize> {
        let pos = (*state)?;
        if pos >= self.0.len() {
            // Already matched the full prefix, accept any continuation.
            Some(pos)
        } else if self.0[pos] == byte {
            Some(pos + 1)
        } else {
            // Mismatch — dead state.
            None
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn empty_index() {
        let index: FstIndex<u32> = FstIndex::build(Vec::<(&str, u32)>::new()).unwrap();
        assert_eq!(index.len(), 0);
        assert!(index.is_empty());
        assert_eq!(index.get(b"anything"), None);
        assert!(index.prefix_values(b"any").is_empty());
    }

    #[test]
    fn single_entry() {
        let index = FstIndex::build([("hello", 42u32)]).unwrap();
        assert_eq!(index.len(), 1);
        assert!(!index.is_empty());
        assert_eq!(index.get(b"hello"), Some(&42));
        assert_eq!(index.get(b"world"), None);
    }

    #[test]
    fn exact_lookup() {
        let index = FstIndex::build([("alpha", 1u32), ("beta", 2), ("gamma", 3)]).unwrap();

        assert_eq!(index.get(b"alpha"), Some(&1));
        assert_eq!(index.get(b"beta"), Some(&2));
        assert_eq!(index.get(b"gamma"), Some(&3));
        assert_eq!(index.get(b"delta"), None);
        assert!(index.contains_key(b"beta"));
        assert!(!index.contains_key(b"delta"));
    }

    #[test]
    fn get_mut() {
        let mut index = FstIndex::build([("a", 1u32), ("b", 2)]).unwrap();
        if let Some(v) = index.get_mut(b"a") {
            *v = 100;
        }
        assert_eq!(index.get(b"a"), Some(&100));
        assert_eq!(index.get(b"b"), Some(&2));
    }

    #[test]
    fn prefix_search() {
        let index = FstIndex::build([
            ("PRIORITY=0", 10u32),
            ("PRIORITY=3", 30),
            ("PRIORITY=6", 60),
            ("SYSLOG_IDENTIFIER=sshd", 100),
            ("SYSLOG_IDENTIFIER=systemd", 200),
            ("_HOSTNAME=server1", 300),
        ])
        .unwrap();

        // Prefix matches
        let values = index.prefix_values(b"PRIORITY=");
        assert_eq!(values, vec![&10, &30, &60]);

        let values = index.prefix_values(b"SYSLOG_IDENTIFIER=");
        assert_eq!(values, vec![&100, &200]);

        let values = index.prefix_values(b"_HOSTNAME=");
        assert_eq!(values, vec![&300]);

        // No matches
        let values = index.prefix_values(b"NONEXISTENT=");
        assert!(values.is_empty());

        // Empty prefix matches everything
        let values = index.prefix_values(b"");
        assert_eq!(values.len(), 6);

        // Full key as prefix returns just that entry
        let values = index.prefix_values(b"PRIORITY=0");
        assert_eq!(values, vec![&10]);
    }

    #[test]
    fn prefix_pairs() {
        let index = FstIndex::build([("aa", 1u32), ("ab", 2), ("ba", 3)]).unwrap();

        let pairs = index.prefix_pairs(b"a");
        assert_eq!(pairs.len(), 2);
        assert_eq!(pairs[0].0, b"aa");
        assert_eq!(pairs[0].1, &1);
        assert_eq!(pairs[1].0, b"ab");
        assert_eq!(pairs[1].1, &2);
    }

    #[test]
    fn prefix_for_each_visits_all() {
        let index = FstIndex::build([("x1", 1u32), ("x2", 2), ("y1", 3)]).unwrap();

        let mut collected = Vec::new();
        index.prefix_for_each(b"x", |key, val| {
            collected.push((key.to_vec(), *val));
        });
        assert_eq!(collected, vec![(b"x1".to_vec(), 1), (b"x2".to_vec(), 2)]);
    }

    #[test]
    fn unsorted_input_is_sorted() {
        let index = FstIndex::build([("cherry", 3u32), ("apple", 1), ("banana", 2)]).unwrap();

        // Values should be in key-sorted order
        assert_eq!(index.values(), &[1, 2, 3]);
        assert_eq!(index.get(b"apple"), Some(&1));
        assert_eq!(index.get(b"banana"), Some(&2));
        assert_eq!(index.get(b"cherry"), Some(&3));
    }

    #[test]
    fn build_from_sorted() {
        let index = FstIndex::build_from_sorted([("a", 1u32), ("b", 2), ("c", 3)]).unwrap();

        assert_eq!(index.len(), 3);
        assert_eq!(index.get(b"a"), Some(&1));
        assert_eq!(index.get(b"c"), Some(&3));
    }

    #[test]
    fn build_from_sorted_rejects_unsorted() {
        let result = FstIndex::build_from_sorted([("b", 1u32), ("a", 2)]);
        assert!(result.is_err());
    }

    #[test]
    fn clone() {
        let index = FstIndex::build([("key", 42u32)]).unwrap();
        let cloned = index.clone();
        assert_eq!(cloned.get(b"key"), Some(&42));
        assert_eq!(cloned.len(), 1);
    }

    #[test]
    fn debug_format() {
        let index = FstIndex::build([("a", 1u32), ("b", 2)]).unwrap();
        let debug = format!("{:?}", index);
        assert!(debug.contains("FstIndex"));
        assert!(debug.contains("len: 2"));
    }

    #[test]
    fn fst_bytes_is_nonzero() {
        let index = FstIndex::build([("a", 1u32)]).unwrap();
        assert!(index.fst_bytes() > 0);
    }

    #[test]
    fn for_each_visits_all() {
        let index = FstIndex::build([("a", 1u32), ("b", 2), ("c", 3)]).unwrap();
        let mut collected = Vec::new();
        index.for_each(|key, val| {
            collected.push((key.to_vec(), *val));
        });
        assert_eq!(
            collected,
            vec![(b"a".to_vec(), 1), (b"b".to_vec(), 2), (b"c".to_vec(), 3),]
        );
    }

    #[cfg(feature = "serde")]
    #[test]
    fn serde_round_trip() {
        let index = FstIndex::build([("alpha", 1u32), ("beta", 2), ("gamma", 3)]).unwrap();

        let json = serde_json::to_string(&index).unwrap();
        let deserialized: FstIndex<u32> = serde_json::from_str(&json).unwrap();

        assert_eq!(deserialized.len(), 3);
        assert_eq!(deserialized.get(b"alpha"), Some(&1));
        assert_eq!(deserialized.get(b"beta"), Some(&2));
        assert_eq!(deserialized.get(b"gamma"), Some(&3));
        assert_eq!(deserialized.get(b"delta"), None);
    }

    #[cfg(feature = "serde")]
    #[test]
    fn serde_round_trip_empty() {
        let index: FstIndex<u32> = FstIndex::build(Vec::<(&str, u32)>::new()).unwrap();

        let json = serde_json::to_string(&index).unwrap();
        let deserialized: FstIndex<u32> = serde_json::from_str(&json).unwrap();

        assert_eq!(deserialized.len(), 0);
        assert!(deserialized.is_empty());
    }
}
