use std::collections::HashSet;
use std::fs;
use std::path::{Path, PathBuf};

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub(super) struct ProcessResidentMappingBreakdown {
    pub(super) heap_bytes: u64,
    pub(super) anon_other_bytes: u64,
    pub(super) journal_raw_bytes: u64,
    pub(super) journal_1m_bytes: u64,
    pub(super) journal_5m_bytes: u64,
    pub(super) journal_1h_bytes: u64,
    pub(super) geoip_asn_bytes: u64,
    pub(super) geoip_geo_bytes: u64,
    pub(super) other_file_bytes: u64,
    pub(super) shmem_bytes: u64,
}

impl ProcessResidentMappingBreakdown {
    #[cfg(test)]
    pub(super) fn total(self) -> u64 {
        self.heap_bytes
            .saturating_add(self.anon_other_bytes)
            .saturating_add(self.journal_raw_bytes)
            .saturating_add(self.journal_1m_bytes)
            .saturating_add(self.journal_5m_bytes)
            .saturating_add(self.journal_1h_bytes)
            .saturating_add(self.geoip_asn_bytes)
            .saturating_add(self.geoip_geo_bytes)
            .saturating_add(self.other_file_bytes)
            .saturating_add(self.shmem_bytes)
    }

    fn add(self, kind: MappingKind, bytes: u64) -> Self {
        let mut next = self;
        match kind {
            MappingKind::Heap => next.heap_bytes = next.heap_bytes.saturating_add(bytes),
            MappingKind::AnonOther => {
                next.anon_other_bytes = next.anon_other_bytes.saturating_add(bytes)
            }
            MappingKind::JournalRaw => {
                next.journal_raw_bytes = next.journal_raw_bytes.saturating_add(bytes)
            }
            MappingKind::Journal1m => {
                next.journal_1m_bytes = next.journal_1m_bytes.saturating_add(bytes)
            }
            MappingKind::Journal5m => {
                next.journal_5m_bytes = next.journal_5m_bytes.saturating_add(bytes)
            }
            MappingKind::Journal1h => {
                next.journal_1h_bytes = next.journal_1h_bytes.saturating_add(bytes)
            }
            MappingKind::GeoIpAsn => {
                next.geoip_asn_bytes = next.geoip_asn_bytes.saturating_add(bytes)
            }
            MappingKind::GeoIpGeo => {
                next.geoip_geo_bytes = next.geoip_geo_bytes.saturating_add(bytes)
            }
            MappingKind::OtherFile => {
                next.other_file_bytes = next.other_file_bytes.saturating_add(bytes)
            }
            MappingKind::Shmem => next.shmem_bytes = next.shmem_bytes.saturating_add(bytes),
        }
        next
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct ProcessResidentMappingPaths {
    raw_tier_prefix: String,
    minute_1_tier_prefix: String,
    minute_5_tier_prefix: String,
    hour_1_tier_prefix: String,
    geoip_asn_paths: HashSet<String>,
    geoip_geo_paths: HashSet<String>,
}

impl ProcessResidentMappingPaths {
    pub(crate) fn new(
        raw_tier_dir: &Path,
        minute_1_tier_dir: &Path,
        minute_5_tier_dir: &Path,
        hour_1_tier_dir: &Path,
        geoip_asn_paths: &[String],
        geoip_geo_paths: &[String],
    ) -> Self {
        Self {
            raw_tier_prefix: journal_tier_prefix(raw_tier_dir),
            minute_1_tier_prefix: journal_tier_prefix(minute_1_tier_dir),
            minute_5_tier_prefix: journal_tier_prefix(minute_5_tier_dir),
            hour_1_tier_prefix: journal_tier_prefix(hour_1_tier_dir),
            geoip_asn_paths: normalized_mapping_path_set(geoip_asn_paths),
            geoip_geo_paths: normalized_mapping_path_set(geoip_geo_paths),
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum MappingKind {
    Heap,
    AnonOther,
    JournalRaw,
    Journal1m,
    Journal5m,
    Journal1h,
    GeoIpAsn,
    GeoIpGeo,
    OtherFile,
    Shmem,
}

pub(super) fn sample_process_resident_mapping_breakdown(
    tier_paths: &ProcessResidentMappingPaths,
) -> ProcessResidentMappingBreakdown {
    let Ok(smaps) = fs::read_to_string("/proc/self/smaps") else {
        return ProcessResidentMappingBreakdown::default();
    };

    parse_process_resident_mapping_breakdown(&smaps, tier_paths)
}

fn parse_process_resident_mapping_breakdown(
    smaps: &str,
    tier_paths: &ProcessResidentMappingPaths,
) -> ProcessResidentMappingBreakdown {
    let mut breakdown = ProcessResidentMappingBreakdown::default();
    let mut current_kind = None;
    let mut current_rss = 0_u64;

    for line in smaps.lines() {
        if let Some(kind) = parse_mapping_kind(line, tier_paths) {
            if let Some(previous) = current_kind.take() {
                breakdown = breakdown.add(previous, current_rss);
            }
            current_kind = Some(kind);
            current_rss = 0;
            continue;
        }

        if let Some(value) = line.strip_prefix("Rss:") {
            current_rss = parse_status_kib(value);
        }
    }

    if let Some(last_kind) = current_kind {
        breakdown = breakdown.add(last_kind, current_rss);
    }

    breakdown
}

fn parse_mapping_kind(
    header_line: &str,
    tier_paths: &ProcessResidentMappingPaths,
) -> Option<MappingKind> {
    let mut parts = header_line.split_whitespace();
    let range = parts.next()?;
    let perms = parts.next()?;

    if !range.contains('-') || perms.len() != 4 {
        return None;
    }

    let _offset = parts.next()?;
    let _dev = parts.next()?;
    let _inode = parts.next()?;
    let path = parts.collect::<Vec<_>>().join(" ");
    Some(classify_mapping_path(path.as_str(), tier_paths))
}

fn classify_mapping_path(path: &str, tier_paths: &ProcessResidentMappingPaths) -> MappingKind {
    let normalized = normalize_mapping_path(path);

    if normalized == "[heap]" {
        return MappingKind::Heap;
    }

    if normalized.starts_with(tier_paths.raw_tier_prefix.as_str()) {
        return MappingKind::JournalRaw;
    }
    if normalized.starts_with(tier_paths.minute_1_tier_prefix.as_str()) {
        return MappingKind::Journal1m;
    }
    if normalized.starts_with(tier_paths.minute_5_tier_prefix.as_str()) {
        return MappingKind::Journal5m;
    }
    if normalized.starts_with(tier_paths.hour_1_tier_prefix.as_str()) {
        return MappingKind::Journal1h;
    }
    if tier_paths.geoip_asn_paths.contains(normalized) {
        return MappingKind::GeoIpAsn;
    }
    if tier_paths.geoip_geo_paths.contains(normalized) {
        return MappingKind::GeoIpGeo;
    }

    if normalized.is_empty() || normalized.starts_with('[') {
        return MappingKind::AnonOther;
    }

    if normalized.starts_with("/SYSV")
        || normalized.starts_with("/memfd:")
        || normalized.starts_with("/dev/shm/")
    {
        return MappingKind::Shmem;
    }

    MappingKind::OtherFile
}

fn normalize_mapping_path(path: &str) -> &str {
    path.trim_end_matches(" (deleted)")
}

fn journal_tier_prefix(path: &Path) -> String {
    let resolved = resolve_monitor_path(path);
    let mut prefix = resolved.to_string_lossy().to_string();
    if !prefix.ends_with('/') {
        prefix.push('/');
    }
    prefix
}

fn normalized_mapping_path_set(paths: &[String]) -> HashSet<String> {
    paths
        .iter()
        .map(|path| normalize_owned_mapping_path(&resolve_monitor_path(Path::new(path))))
        .collect()
}

fn normalize_owned_mapping_path(path: &Path) -> String {
    normalize_mapping_path(path.to_string_lossy().as_ref()).to_string()
}

fn resolve_monitor_path(path: &Path) -> PathBuf {
    let absolute = if path.is_absolute() {
        path.to_path_buf()
    } else {
        std::env::current_dir()
            .map(|cwd| cwd.join(path))
            .unwrap_or_else(|_| path.to_path_buf())
    };

    fs::canonicalize(&absolute).unwrap_or(absolute)
}

fn parse_status_kib(raw: &str) -> u64 {
    raw.split_whitespace()
        .next()
        .and_then(|value| value.parse::<u64>().ok())
        .unwrap_or(0)
        .saturating_mul(1024)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn tier_paths() -> ProcessResidentMappingPaths {
        ProcessResidentMappingPaths::new(
            Path::new("/var/cache/netdata/flows/raw"),
            Path::new("/var/cache/netdata/flows/1m"),
            Path::new("/var/cache/netdata/flows/5m"),
            Path::new("/var/cache/netdata/flows/1h"),
            &["/var/cache/netdata/topology-ip-intel/topology-ip-asn.mmdb".to_string()],
            &["/usr/share/netdata/topology-ip-intel/topology-ip-geo.mmdb".to_string()],
        )
    }

    #[test]
    fn parse_process_resident_mapping_breakdown_classifies_disjoint_mapping_buckets() {
        let breakdown = parse_process_resident_mapping_breakdown(
            r#"55c1ce28b000-55c1de41e000 rw-p 00000000 00:00 0 [heap]
Rss:               25712 kB
7fea4b175000-7fea538bf000 rw-p 00000000 00:00 0
Rss:              138536 kB
7fea49e00000-7fea4a600000 rw-s 00000000 103:02 125872517 /var/cache/netdata/flows/raw/node/system@abc.journal
Rss:                1516 kB
7fea49cff000-7fea49e00000 rw-s 00000000 103:02 125872521 /var/cache/netdata/flows/1m/node/system@def.journal
Rss:                1028 kB
7fea49400000-7fea49c00000 rw-s 00000000 103:02 125872522 /var/cache/netdata/flows/5m/node/system@ghi.journal (deleted)
Rss:                 620 kB
7fea55078000-7fea55079000 rw-s 00000000 103:02 125872523 /var/cache/netdata/flows/1h/node/system@jkl.journal
Rss:                  64 kB
7fea56000000-7fea56a00000 r--p 00000000 103:02 125872524 /var/cache/netdata/topology-ip-intel/topology-ip-asn.mmdb
Rss:                 320 kB
7fea57000000-7fea57f00000 r--p 00000000 103:02 125872525 /usr/share/netdata/topology-ip-intel/topology-ip-geo.mmdb
Rss:                 960 kB
55c198157000-55c198c4d000 r-xp 00336000 103:02 176030973 /usr/libexec/netdata/plugins.d/netflow-plugin
Rss:                7316 kB
7fea60000000-7fea60010000 rw-s 00000000 00:00 0 /SYSV00000000 (deleted)
Rss:                 128 kB
"#,
            &tier_paths(),
        );

        assert_eq!(breakdown.heap_bytes, 25_712 * 1024);
        assert_eq!(breakdown.anon_other_bytes, 138_536 * 1024);
        assert_eq!(breakdown.journal_raw_bytes, 1_516 * 1024);
        assert_eq!(breakdown.journal_1m_bytes, 1_028 * 1024);
        assert_eq!(breakdown.journal_5m_bytes, 620 * 1024);
        assert_eq!(breakdown.journal_1h_bytes, 64 * 1024);
        assert_eq!(breakdown.geoip_asn_bytes, 320 * 1024);
        assert_eq!(breakdown.geoip_geo_bytes, 960 * 1024);
        assert_eq!(breakdown.other_file_bytes, 7_316 * 1024);
        assert_eq!(breakdown.shmem_bytes, 128 * 1024);
        assert_eq!(
            breakdown.total(),
            (25_712 + 138_536 + 1_516 + 1_028 + 620 + 64 + 320 + 960 + 7_316 + 128) * 1024
        );
    }

    #[test]
    fn relative_tier_dirs_are_resolved_under_current_working_directory() {
        let cwd = std::env::current_dir().expect("current dir");
        let paths = ProcessResidentMappingPaths::new(
            Path::new("flows/raw"),
            Path::new("flows/1m"),
            Path::new("flows/5m"),
            Path::new("flows/1h"),
            &["topology-ip-intel/topology-ip-asn.mmdb".to_string()],
            &["topology-ip-intel/topology-ip-geo.mmdb".to_string()],
        );

        assert_eq!(
            paths.raw_tier_prefix,
            format!("{}/flows/raw/", cwd.display())
        );
        assert_eq!(
            paths.minute_1_tier_prefix,
            format!("{}/flows/1m/", cwd.display())
        );
        assert_eq!(
            paths.minute_5_tier_prefix,
            format!("{}/flows/5m/", cwd.display())
        );
        assert_eq!(
            paths.hour_1_tier_prefix,
            format!("{}/flows/1h/", cwd.display())
        );
        assert_eq!(
            paths.geoip_asn_paths,
            HashSet::from([format!(
                "{}/topology-ip-intel/topology-ip-asn.mmdb",
                cwd.display()
            )])
        );
        assert_eq!(
            paths.geoip_geo_paths,
            HashSet::from([format!(
                "{}/topology-ip-intel/topology-ip-geo.mmdb",
                cwd.display()
            )])
        );
    }
}
