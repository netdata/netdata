//! Closed minutes on disk: one file per tenant and hour, rewritten whole by
//! write-then-rename, so a reader sees the old or the new file and a crash
//! leaves at most a stray temp file. The format belongs to this crate only
//! and carries no compatibility promise: a file this version cannot read is
//! skipped and counted, and a future version may drop the directory.
//!
//! File layout: magic `NDSM`, format version (u8), payload length (u32 LE),
//! CRC32 of the payload (u32 LE), payload (bincode of `HourFile`).

use std::collections::{BTreeMap, HashMap};
use std::fs;
use std::io::{self, Write};
use std::path::{Path, PathBuf};
use std::sync::Arc;

use serde::{Deserialize, Serialize};

use crate::engine::ClosedMinute;
use crate::stats::MinuteStats;

const MAGIC: &[u8; 4] = b"NDSM";
// Bumped on every change of `HourFile`'s encoding: an older file then reads
// as corrupt instead of decoding into the wrong fields.
const VERSION: u8 = 2;
const HEADER_LEN: usize = 4 + 1 + 4 + 4;
const EXTENSION: &str = "smh";
const MINUTES_PER_HOUR: u64 = 60;

#[derive(Debug, Default, Serialize, Deserialize)]
struct HourFile {
    minutes: BTreeMap<u64, MinuteStats>,
}

#[derive(Clone, Debug)]
pub struct Retention {
    /// Hours older than this are deleted.
    pub max_age_minutes: u64,
    /// Oldest hours are deleted while the directory is larger than this.
    pub max_bytes: u64,
}

impl Default for Retention {
    fn default() -> Self {
        Self {
            max_age_minutes: 7 * 24 * 60,
            max_bytes: 256 * 1024 * 1024,
        }
    }
}

pub struct Store {
    root: PathBuf,
    retention: Retention,
}

/// Minutes read for a window, merged per minute.
#[derive(Debug, Default)]
pub struct Loaded {
    pub minutes: BTreeMap<u64, MinuteStats>,
    /// Files skipped because they could not be read or did not verify.
    pub unreadable_files: u64,
}

impl Store {
    pub fn open(root: impl Into<PathBuf>, retention: Retention) -> io::Result<Self> {
        let root = root.into();
        fs::create_dir_all(&root)?;
        Ok(Self { root, retention })
    }

    /// Adds closed minutes to their hour files. A minute already on disk (from
    /// before a restart) is merged, not replaced. A file that cannot be
    /// written does not stop the others; the minutes that could not be
    /// written are returned per tenant, so they can be reported as lost.
    pub fn write(&self, closed: Vec<ClosedMinute>) -> Vec<(Arc<str>, u64)> {
        type FileKey = (Arc<str>, u64);
        let mut by_file: HashMap<FileKey, Vec<(u64, MinuteStats)>> = HashMap::new();
        for c in closed {
            by_file
                .entry((c.tenant, c.minute / MINUTES_PER_HOUR))
                .or_default()
                .push((c.minute, c.stats));
        }
        let mut failed: BTreeMap<Arc<str>, u64> = BTreeMap::new();
        for ((tenant, hour), minutes) in by_file {
            if let Err(e) = self.write_hour(&tenant, hour, &minutes) {
                tracing::error!(%tenant, hour, %e, "service map: cannot write closed minutes; they are lost");
                *failed.entry(tenant).or_insert(0) += minutes.len() as u64;
            }
        }
        failed.into_iter().collect()
    }

    fn write_hour(
        &self,
        tenant: &str,
        hour: u64,
        minutes: &[(u64, MinuteStats)],
    ) -> io::Result<()> {
        let dir = self.root.join(tenant_dir(tenant));
        fs::create_dir_all(&dir)?;
        let path = dir.join(format!("{hour}.{EXTENSION}"));
        let mut file = match read_file(&path)? {
            Read::Missing => HourFile::default(),
            // Its minutes are already lost (and counted on every load); the
            // new ones still get a readable file.
            Read::Corrupt => HourFile::default(),
            Read::Ok(file) => file,
        };
        for (minute, stats) in minutes {
            file.minutes.entry(*minute).or_default().merge(stats);
        }
        write_file(&path, &file)
    }

    /// The stored minutes of `tenant` in `[from_minute, to_minute]`.
    pub fn load(&self, tenant: &str, from_minute: u64, to_minute: u64) -> Loaded {
        let mut loaded = Loaded::default();
        if from_minute > to_minute {
            return loaded;
        }
        let dir = self.root.join(tenant_dir(tenant));
        for hour in from_minute / MINUTES_PER_HOUR..=to_minute / MINUTES_PER_HOUR {
            let path = dir.join(format!("{hour}.{EXTENSION}"));
            let file = match read_file(&path) {
                Ok(Read::Missing) => continue,
                Ok(Read::Ok(file)) => file,
                Ok(Read::Corrupt) | Err(_) => {
                    loaded.unreadable_files += 1;
                    continue;
                }
            };
            for (minute, stats) in file.minutes.range(from_minute..=to_minute) {
                loaded.minutes.entry(*minute).or_default().merge(stats);
            }
        }
        loaded
    }

    /// Deletes hour files older than the age limit, then the oldest hours
    /// until the store fits the size limit, plus stray temp files.
    pub fn enforce_retention(&self, now_minute: u64) -> io::Result<()> {
        let oldest_kept_hour =
            now_minute.saturating_sub(self.retention.max_age_minutes) / MINUTES_PER_HOUR;
        let mut kept: Vec<(u64, u64, PathBuf)> = Vec::new();
        let mut total: u64 = 0;
        for tenant in fs::read_dir(&self.root)? {
            let tenant = tenant?;
            if !tenant.file_type()?.is_dir() {
                continue;
            }
            for entry in fs::read_dir(tenant.path())? {
                let entry = entry?;
                let path = entry.path();
                let Some(hour) = hour_of(&path) else {
                    if path.extension().is_some_and(|e| e == "tmp") {
                        remove_quietly(&path);
                    }
                    continue;
                };
                if hour < oldest_kept_hour {
                    remove_quietly(&path);
                    continue;
                }
                let size = entry.metadata()?.len();
                total += size;
                kept.push((hour, size, path));
            }
        }
        kept.sort_by_key(|(hour, _, _)| *hour);
        for (_, size, path) in kept {
            if total <= self.retention.max_bytes {
                break;
            }
            remove_quietly(&path);
            total -= size;
        }
        Ok(())
    }
}

/// A file-system-safe directory name per tenant; tenant ids outside the
/// plain set are hex-encoded so two tenants never share a directory.
fn tenant_dir(tenant: &str) -> String {
    let plain = !tenant.is_empty()
        && !tenant.starts_with('.')
        && tenant
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'-' || b == b'_' || b == b'.');
    if plain {
        return format!("t-{tenant}");
    }
    let mut out = String::from("x-");
    for b in tenant.bytes() {
        out.push_str(&format!("{b:02x}"));
    }
    out
}

fn hour_of(path: &Path) -> Option<u64> {
    if path.extension()? != EXTENSION {
        return None;
    }
    path.file_stem()?.to_str()?.parse().ok()
}

fn remove_quietly(path: &Path) {
    if let Err(e) = fs::remove_file(path) {
        if e.kind() != io::ErrorKind::NotFound {
            tracing::warn!(path = %path.display(), %e, "service map: cannot remove file");
        }
    }
}

enum Read {
    Missing,
    /// Wrong magic, version, length or checksum, or a payload that does not
    /// decode: a file from another format version or a damaged one.
    Corrupt,
    Ok(HourFile),
}

fn read_file(path: &Path) -> io::Result<Read> {
    let bytes = match fs::read(path) {
        Ok(bytes) => bytes,
        Err(e) if e.kind() == io::ErrorKind::NotFound => return Ok(Read::Missing),
        Err(e) => return Err(e),
    };
    Ok(decode(&bytes).map_or(Read::Corrupt, Read::Ok))
}

fn decode(bytes: &[u8]) -> Option<HourFile> {
    if bytes.len() < HEADER_LEN || &bytes[..4] != MAGIC || bytes[4] != VERSION {
        return None;
    }
    let len = u32::from_le_bytes(bytes[5..9].try_into().ok()?) as usize;
    let crc = u32::from_le_bytes(bytes[9..13].try_into().ok()?);
    let payload = bytes.get(HEADER_LEN..HEADER_LEN + len)?;
    if bytes.len() != HEADER_LEN + len || crc32fast::hash(payload) != crc {
        return None;
    }
    let (file, _) = bincode::serde::decode_from_slice(payload, bincode::config::standard()).ok()?;
    Some(file)
}

fn write_file(path: &Path, file: &HourFile) -> io::Result<()> {
    let payload = bincode::serde::encode_to_vec(file, bincode::config::standard())
        .map_err(io::Error::other)?;
    let len = u32::try_from(payload.len()).map_err(io::Error::other)?;
    let tmp = path.with_extension("tmp");
    {
        let mut out = fs::File::create(&tmp)?;
        out.write_all(MAGIC)?;
        out.write_all(&[VERSION])?;
        out.write_all(&len.to_le_bytes())?;
        out.write_all(&crc32fast::hash(&payload).to_le_bytes())?;
        out.write_all(&payload)?;
        out.sync_all()?;
    }
    fs::rename(&tmp, path)?;
    // Make the rename itself durable.
    if let Some(dir) = path.parent() {
        fs::File::open(dir)?.sync_all()?;
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::facts::ServiceKey;
    use crate::stats::{Edge, EdgeStats, Node};

    fn minute_with(calls: u64, late: u64) -> MinuteStats {
        let mut stats = MinuteStats::default();
        let edge = Edge {
            caller: Node::UnknownCaller,
            callee: Node::Service(ServiceKey {
                namespace: None,
                name: Arc::from("a"),
            }),
        };
        let mut edge_stats = EdgeStats::default();
        for _ in 0..calls {
            edge_stats.server.record(1_000, false);
        }
        stats.edges.insert(edge, edge_stats);
        stats.losses.late = late;
        stats
    }

    fn closed(tenant: &str, minute: u64, stats: MinuteStats) -> ClosedMinute {
        ClosedMinute {
            tenant: Arc::from(tenant),
            minute,
            stats,
        }
    }

    fn calls(loaded: &Loaded) -> Vec<(u64, u64)> {
        let mut out = Vec::new();
        for (minute, stats) in &loaded.minutes {
            out.push((*minute, stats.edges.values().map(|e| e.server.calls).sum()));
        }
        out
    }

    #[test]
    fn round_trip_and_window() {
        let dir = tempfile::tempdir().unwrap();
        let store = Store::open(dir.path(), Retention::default()).unwrap();
        let failed = store.write(vec![
            closed("default", 119, minute_with(1, 0)),
            closed("default", 120, minute_with(2, 0)),
            closed("default", 185, minute_with(3, 0)),
            closed("other", 120, minute_with(9, 0)),
        ]);
        assert!(failed.is_empty());
        let loaded = store.load("default", 120, 200);
        assert_eq!(calls(&loaded), vec![(120, 2), (185, 3)]);
        assert_eq!(loaded.unreadable_files, 0);
        assert_eq!(
            calls(&store.load("default", 0, 1_000)),
            vec![(119, 1), (120, 2), (185, 3)]
        );
        assert!(store.load("missing", 0, 1_000).minutes.is_empty());
        assert!(store.load("default", 10, 5).minutes.is_empty());
    }

    #[test]
    fn a_minute_written_twice_is_merged() {
        let dir = tempfile::tempdir().unwrap();
        let store = Store::open(dir.path(), Retention::default()).unwrap();
        assert!(
            store
                .write(vec![closed("default", 60, minute_with(2, 1))])
                .is_empty()
        );
        assert!(
            store
                .write(vec![closed("default", 60, minute_with(3, 4))])
                .is_empty()
        );
        let loaded = store.load("default", 60, 60);
        assert_eq!(calls(&loaded), vec![(60, 5)]);
        assert_eq!(loaded.minutes[&60].losses.late, 5);
    }

    #[test]
    fn corrupt_files_are_skipped_and_counted_and_temp_files_ignored() {
        let dir = tempfile::tempdir().unwrap();
        let store = Store::open(dir.path(), Retention::default()).unwrap();
        assert!(
            store
                .write(vec![
                    closed("default", 60, minute_with(1, 0)),
                    closed("default", 120, minute_with(1, 0)),
                ])
                .is_empty()
        );
        let tenant = dir.path().join(tenant_dir("default"));
        let mut bytes = fs::read(tenant.join("1.smh")).unwrap();
        let last = bytes.len() - 1;
        bytes[last] ^= 0xff;
        fs::write(tenant.join("1.smh"), bytes).unwrap();
        fs::write(tenant.join("2.tmp"), b"partial").unwrap();
        let loaded = store.load("default", 0, 1_000);
        assert_eq!(calls(&loaded), vec![(120, 1)]);
        assert_eq!(loaded.unreadable_files, 1);
    }

    #[test]
    fn a_failing_file_does_not_drop_the_others() {
        let dir = tempfile::tempdir().unwrap();
        let store = Store::open(dir.path(), Retention::default()).unwrap();
        // The tenant's directory cannot be created: a file has its name.
        fs::write(dir.path().join(tenant_dir("blocked")), b"").unwrap();
        let failed = store.write(vec![
            closed("blocked", 60, minute_with(1, 0)),
            closed("blocked", 61, minute_with(1, 0)),
            closed("a", 60, minute_with(2, 0)),
            closed("b", 120, minute_with(3, 0)),
        ]);
        assert_eq!(failed, vec![(Arc::from("blocked"), 2)]);
        assert_eq!(calls(&store.load("a", 0, 1_000)), vec![(60, 2)]);
        assert_eq!(calls(&store.load("b", 0, 1_000)), vec![(120, 3)]);
    }

    #[test]
    fn an_unreadable_hour_file_is_not_overwritten_by_an_io_error() {
        let dir = tempfile::tempdir().unwrap();
        let store = Store::open(dir.path(), Retention::default()).unwrap();
        assert!(
            store
                .write(vec![closed("default", 60, minute_with(1, 0))])
                .is_empty()
        );
        let path = dir.path().join(tenant_dir("default")).join("1.smh");
        // A directory where the hour file should be: reading fails with an
        // I/O error, not "missing" or "corrupt".
        fs::remove_file(&path).unwrap();
        fs::create_dir(&path).unwrap();
        let failed = store.write(vec![closed("default", 61, minute_with(1, 0))]);
        assert_eq!(failed, vec![(Arc::from("default"), 1)]);
        assert!(path.is_dir(), "left as it was");
    }

    #[test]
    fn retention_drops_old_hours_then_the_oldest_until_it_fits() {
        let dir = tempfile::tempdir().unwrap();
        let mut closed_minutes = Vec::new();
        for hour in 0..5u64 {
            closed_minutes.push(closed("default", hour * 60, minute_with(1, 0)));
        }
        let store = Store::open(dir.path(), Retention::default()).unwrap();
        assert!(store.write(closed_minutes).is_empty());
        let tenant = dir.path().join(tenant_dir("default"));
        let file_size = fs::metadata(tenant.join("4.smh")).unwrap().len();
        fs::write(tenant.join("9.tmp"), b"partial").unwrap();

        let store = Store::open(
            dir.path(),
            Retention {
                max_age_minutes: 4 * 60,
                max_bytes: 2 * file_size,
            },
        )
        .unwrap();
        // Now at hour 4: hour 0 is too old; of hours 1-4 the two oldest go.
        store.enforce_retention(4 * 60 + 30).unwrap();
        let mut left: Vec<String> = fs::read_dir(&tenant)
            .unwrap()
            .map(|e| e.unwrap().file_name().into_string().unwrap())
            .collect();
        left.sort();
        assert_eq!(left, vec!["3.smh", "4.smh"]);
    }

    #[test]
    fn tenant_directories_are_safe_and_distinct() {
        assert_eq!(tenant_dir("default"), "t-default");
        assert_eq!(tenant_dir("../x"), "x-2e2e2f78");
        assert_eq!(tenant_dir(".hidden"), "x-2e68696464656e");
        assert_ne!(tenant_dir("a/b"), tenant_dir("a_b"));
    }
}
