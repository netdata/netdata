use super::*;
use ipnet_trie::IpnetTrie;

#[derive(Debug, Clone)]
pub(crate) struct PrefixMapEntry<T> {
    pub(crate) prefix: IpNet,
    pub(crate) value: T,
}

/// IP prefix map optimized for longest-prefix-match lookups.
///
/// Entries are separated by address family (IPv4 vs IPv6) and sorted by prefix length
/// descending. Lookup returns the first match, which is the longest prefix match.
/// This avoids scanning entries of the wrong address family and terminates early.
#[derive(Clone)]
pub(crate) struct PrefixMap<T> {
    // Sorted by prefix_len descending after finalize() for longest-match-first.
    pub(crate) v4_entries: Vec<PrefixMapEntry<T>>,
    pub(crate) v6_entries: Vec<PrefixMapEntry<T>>,
    lookup_index: IpnetTrie<PrefixLookupRef>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum PrefixLookupRef {
    V4(usize),
    V6(usize),
}

impl<T> Default for PrefixMap<T> {
    fn default() -> Self {
        Self {
            v4_entries: Vec::new(),
            v6_entries: Vec::new(),
            lookup_index: IpnetTrie::new(),
        }
    }
}

impl<T: std::fmt::Debug> std::fmt::Debug for PrefixMap<T> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("PrefixMap")
            .field("v4_entries", &self.v4_entries)
            .field("v6_entries", &self.v6_entries)
            .field("lookup_index", &"<IpnetTrie>")
            .finish()
    }
}

impl<T> PrefixMap<T> {
    pub(crate) fn insert(&mut self, prefix: IpNet, value: T) {
        let entry = PrefixMapEntry { prefix, value };
        match prefix {
            IpNet::V4(_) => self.v4_entries.push(entry),
            IpNet::V6(_) => self.v6_entries.push(entry),
        }
    }

    /// Sort entries by prefix length descending. Must be called after all inserts.
    pub(crate) fn finalize(&mut self) {
        self.v4_entries
            .sort_by(|a, b| b.prefix.prefix_len().cmp(&a.prefix.prefix_len()));
        self.v6_entries
            .sort_by(|a, b| b.prefix.prefix_len().cmp(&a.prefix.prefix_len()));

        let mut lookup_index = IpnetTrie::new();
        for (index, entry) in self.v4_entries.iter().enumerate() {
            lookup_index.insert(entry.prefix, PrefixLookupRef::V4(index));
        }
        for (index, entry) in self.v6_entries.iter().enumerate() {
            lookup_index.insert(entry.prefix, PrefixLookupRef::V6(index));
        }
        self.lookup_index = lookup_index;
    }

    pub(crate) fn is_empty(&self) -> bool {
        self.v4_entries.is_empty() && self.v6_entries.is_empty()
    }

    /// Longest-prefix-match: returns the value associated with the most specific prefix.
    /// Uses a trie-backed index for direct lookups while preserving the sorted vectors for
    /// ordered multi-match walks.
    pub(crate) fn lookup(&self, address: IpAddr) -> Option<&T> {
        let host_prefix = IpNet::from(address);
        let (_, entry_ref) = self.lookup_index.longest_match(&host_prefix)?;
        match entry_ref {
            PrefixLookupRef::V4(index) => self.v4_entries.get(*index).map(|entry| &entry.value),
            PrefixLookupRef::V6(index) => self.v6_entries.get(*index).map(|entry| &entry.value),
        }
    }

    /// Iterate all matching entries in ascending prefix length order (least specific first).
    /// Used by resolve_network_attributes where all matching prefixes must be merged.
    pub(crate) fn matching_entries_ascending(
        &self,
        address: IpAddr,
    ) -> impl Iterator<Item = &PrefixMapEntry<T>> {
        let entries = match address {
            IpAddr::V4(_) => &self.v4_entries,
            IpAddr::V6(_) => &self.v6_entries,
        };
        // Entries are sorted descending, so reverse iteration gives ascending order.
        entries
            .iter()
            .rev()
            .filter(move |entry| entry.prefix.contains(&address))
    }
}

pub(crate) fn build_sampling_map(
    sampling: Option<&SamplingRateSetting>,
    field_name: &str,
) -> Result<PrefixMap<u64>> {
    let mut out = PrefixMap::default();
    let Some(sampling) = sampling else {
        return Ok(out);
    };

    match sampling {
        SamplingRateSetting::Single(rate) => {
            out.insert(parse_prefix("0.0.0.0/0")?, *rate);
            out.insert(parse_prefix("::/0")?, *rate);
        }
        SamplingRateSetting::PerPrefix(entries) => {
            for (prefix, rate) in entries {
                let parsed_prefix = parse_prefix(prefix)
                    .with_context(|| format!("{field_name}: invalid sampling prefix '{prefix}'"))?;
                out.insert(parsed_prefix, *rate);
            }
        }
    }

    out.finalize();
    Ok(out)
}

pub(crate) fn parse_prefix(prefix: &str) -> Result<IpNet> {
    IpNet::from_str(prefix).with_context(|| format!("invalid prefix '{prefix}'"))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn lookup_returns_longest_prefix_match() {
        let mut map = PrefixMap::default();
        map.insert(parse_prefix("198.51.0.0/16").expect("prefix"), "region");
        map.insert(parse_prefix("198.51.100.0/24").expect("prefix"), "site");
        map.finalize();

        assert_eq!(
            map.lookup("198.51.100.42".parse().expect("address")),
            Some(&"site")
        );
    }

    #[test]
    fn matching_entries_ascending_keeps_less_specific_prefix_first() {
        let mut map = PrefixMap::default();
        map.insert(parse_prefix("198.51.0.0/16").expect("prefix"), "region");
        map.insert(parse_prefix("198.51.100.0/24").expect("prefix"), "site");
        map.finalize();

        let matches: Vec<_> = map
            .matching_entries_ascending("198.51.100.42".parse().expect("address"))
            .map(|entry| entry.value)
            .collect();

        assert_eq!(matches, vec!["region", "site"]);
    }
}
