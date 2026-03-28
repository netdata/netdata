use super::*;

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
#[derive(Debug, Clone, Default)]
pub(crate) struct PrefixMap<T> {
    // Sorted by prefix_len descending after finalize() for longest-match-first.
    pub(crate) v4_entries: Vec<PrefixMapEntry<T>>,
    pub(crate) v6_entries: Vec<PrefixMapEntry<T>>,
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
    }

    pub(crate) fn is_empty(&self) -> bool {
        self.v4_entries.is_empty() && self.v6_entries.is_empty()
    }

    /// Longest-prefix-match: returns the value associated with the most specific prefix.
    /// O(n) worst case but terminates on first match due to descending sort order.
    pub(crate) fn lookup(&self, address: IpAddr) -> Option<&T> {
        let entries = match address {
            IpAddr::V4(_) => &self.v4_entries,
            IpAddr::V6(_) => &self.v6_entries,
        };
        for entry in entries {
            if entry.prefix.contains(&address) {
                return Some(&entry.value);
            }
        }
        None
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
