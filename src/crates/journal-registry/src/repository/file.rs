//! The repository's path data model: journal file paths parsed into `File`,
//! plus the recursive scanner that feeds `Registry`'s initial scan.
//!
//! `File` (path + `Origin` + `Status` in an `Arc`) is the unit everything
//! else is built on: `Chain`/`Repository` order and group it
//! (repository/collection.rs), `Registry` pairs it with `TimeRange` metadata
//! ([`crate::FileInfo`]), and the journal stack passes it around as
//! a key — `Eq`/`Hash` cover the full path, origin and status, which
//! journal-engine keys its index cache on (`journal-engine/src/cache.rs`).
//!
//! Naming scheme parsed right to left (`File::from_str`), by suffix first:
//!
//! - `.journal` — the live file (`Status::Active`). With an `@` part before
//!   the suffix it is archived:
//!   `system@<seqnum_id>-<head_seqnum>-<head_realtime>.journal`, with
//!   `seqnum_id` a UUID (the writer emits its 32-hex-char simple form) and
//!   the numbers hex u64s (writer side:
//!   `journal-log-writer/src/log/chain.rs`).
//! - `.journal~` — disposed: `system@<timestamp>-<number>.journal~`, both
//!   hex u64s; a corrupted file renamed with the `~` suffix (README.md).
//! - source basename: `system`, `user-<uid>` (u32), `remote-<host>`, or
//!   anything else → `Unknown`. The basename must sit in a directory.
//! - machine-id directory: the basename's directory, named either
//!   `<machine-id>` or `<machine-id>.<namespace>` (journal namespace). A
//!   plain dirname that is not a UUID simply leaves the machine id unset;
//!   a dotted one whose machine-id half does not parse rejects the file.
//!
//! Every parse mismatch is a silent `None` — path errors never become
//! `RepositoryError` ([`RepositoryError::InvalidPath`] has no producing
//! call site). The errors that do exist come from the scan
//! (`WalkDir`) and from `File::dir()` (`InvalidUtf8`).
//!
//! Ordering: `Status`'s `Ord` puts disposed → archived (by head_realtime,
//! the rotation boundary) → active, which is what keeps the chains in
//! rotation order for the range-coverage model
//! ([`crate::repository::Chain::find_files_in_range`]);
//! `File`'s `Ord` adds a path tiebreak. `File::dir()` — the
//! machine-id directory's parent when the machine-id parsed, else the file's
//! immediate parent — is the key [`crate::repository::Repository`] groups
//! chains by and what [`crate::Registry::unwatch_directory`]
//! matches against.
//!
//! [`crate::repository::file::scan_journal_files`] is re-exported
//! crate-internally (`repository/mod.rs`) and is the initial scan of
//! [`crate::Registry::watch_directory`].
//!
//! Consumers: journal-index (`journal-index/src/file_indexer.rs` —
//! `is_active` guards the header-state check), journal-engine
//! (`journal-engine/src/cache.rs`, `journal-engine/src/logs/query.rs`),
//! journal-log-writer (builds these names and parses them back,
//! `journal-log-writer/src/log/chain.rs`), the journal-core repository
//! re-export (`journal-core/src/lib.rs`) and journal-function's re-export
//! (`journal-function/src/lib.rs`).
//! netflow-plugin runs the same API through the published twin
//! `journal-sdk-registry`. The `allocative` feature adds memory-profiling
//! derives, skipping the `Uuid` fields (the uuid type has no Allocative
//! impl).
use crate::repository::RepositoryError;
use crate::repository::error::Result;
use serde::{Deserialize, Serialize};
use std::cmp::Ordering;
use std::path::Path;
use std::sync::Arc;
use uuid::Uuid;

/// Rotation lifecycle position of a journal file, parsed from the file name
/// (`Status::parse`). The `Ord` impl below defines the chain order the
/// repository relies on — module docs for the whole scheme.
#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub enum Status {
    /// The live journal file, currently being written to.
    Active,
    /// Archived (rotated) journal file, no longer being written to.
    Archived {
        /// Identifies the seqnum series this file's seqnums belong to;
        /// stable across the files of one journal, written as its
        /// 32-hex-char simple form.
        #[cfg_attr(feature = "allocative", allocative(skip))]
        seqnum_id: Uuid,
        /// Seqnum of the file's first entry (hex u64 in the file name).
        head_seqnum: u64,
        /// Realtime timestamp of the first entry, microseconds since epoch —
        /// the chain's rotation-boundary key (`Ord` and
        /// [`crate::repository::Chain::find_files_in_range`] use it).
        head_realtime: u64,
    },
    /// Disposed journal file — corrupted or incomplete, renamed with the
    /// `~` suffix — awaiting cleanup. Its entry bounds are unknowable, so
    /// range queries skip it
    /// ([`crate::repository::Chain::find_files_in_range`]).
    Disposed {
        /// When the file was disposed, microseconds since epoch (hex in the
        /// file name).
        timestamp: u64,
        /// Disambiguates several files disposed at the same timestamp (hex;
        /// also the `Ord` tiebreak).
        number: u64,
    },
}

impl Ord for Status {
    fn cmp(&self, other: &Self) -> Ordering {
        match (self, other) {
            // Disposed first, ordered by timestamp then number.
            (
                Status::Disposed {
                    timestamp: t1,
                    number: n1,
                },
                Status::Disposed {
                    timestamp: t2,
                    number: n2,
                },
            ) => t1.cmp(t2).then_with(|| n1.cmp(n2)),

            // Disposed sorts before everything else.
            (Status::Disposed { .. }, _) => Ordering::Less,
            (_, Status::Disposed { .. }) => Ordering::Greater,

            // Archived: by head_realtime (the rotation boundary), then
            // seqnum_id and head_seqnum as total-order tiebreaks.
            (
                Status::Archived {
                    seqnum_id: lhs_seqnum_id,
                    head_seqnum: lhs_head_seqnum,
                    head_realtime: lhs_head_realtime,
                },
                Status::Archived {
                    seqnum_id: rhs_seqnum_id,
                    head_seqnum: rhs_head_seqnum,
                    head_realtime: rhs_head_realtime,
                },
            ) => lhs_head_realtime
                .cmp(rhs_head_realtime)
                .then_with(|| lhs_seqnum_id.cmp(rhs_seqnum_id))
                .then_with(|| lhs_head_seqnum.cmp(rhs_head_seqnum)),

            // Archived before active: the live file sorts last (the chain
            // scans stop there, `Chain::find_files_in_range` in
            // repository/collection.rs).
            (Status::Archived { .. }, Status::Active) => Ordering::Less,
            (Status::Active, Status::Archived { .. }) => Ordering::Greater,

            // All active files compare Equal.
            (Status::Active, Status::Active) => Ordering::Equal,
        }
    }
}

impl PartialOrd for Status {
    fn partial_cmp(&self, other: &Self) -> Option<Ordering> {
        Some(self.cmp(other))
    }
}

impl Status {
    /// Splits the status suffix off the end of `path`, returning the status
    /// and the remaining path (the name with the suffix — and its `@…`
    /// payload, if any — removed).
    ///
    /// Recognizes exactly three shapes: `<stem>.journal` (active),
    /// `<stem>@<seqnum_id>-<head_seqnum>-<head_realtime>.journal` (archived;
    /// `seqnum_id` a UUID, the numbers hex u64), and
    /// `<stem>@<timestamp>-<number>.journal~` (disposed; both hex). Anything
    /// else is `None` — including a bare `<name>.journal~` with no `@` part,
    /// an archived suffix with other than three fields, or fields that fail
    /// to parse.
    pub(super) fn parse(path: &str) -> Option<(Self, &str)> {
        if let Some(stem) = path.strip_suffix(".journal") {
            // An @ part marks the file archived; without one it is the live
            // file.
            if let Some((prefix, suffix)) = stem.rsplit_once('@') {
                // Archived suffix: @<seqnum_id>-<head_seqnum>-<head_realtime>.
                let mut parts = suffix.split('-');

                let seqnum_id = parts.next()?;
                let head_seqnum = parts.next()?;
                let head_realtime = parts.next()?;

                if parts.next().is_some() {
                    return None; // more than three fields
                }

                let seqnum_id = Uuid::try_parse(seqnum_id).ok()?;
                let head_seqnum = u64::from_str_radix(head_seqnum, 16).ok()?;
                let head_realtime = u64::from_str_radix(head_realtime, 16).ok()?;

                Some((
                    Status::Archived {
                        seqnum_id,
                        head_seqnum,
                        head_realtime,
                    },
                    prefix,
                ))
            } else {
                // No @ part: the live file.
                Some((Status::Active, stem))
            }
        } else if let Some(stem) = path.strip_suffix(".journal~") {
            // Disposed suffix: @<timestamp>-<number>; the @ part is required.
            let (prefix, suffix) = stem.rsplit_once('@')?;
            let (timestamp, number) = suffix.rsplit_once('-')?;

            let timestamp = u64::from_str_radix(timestamp, 16).ok()?;
            let number = u64::from_str_radix(number, 16).ok()?;

            Some((Status::Disposed { timestamp, number }, prefix))
        } else {
            None
        }
    }
}

/// Which stream a journal file belongs to, classified from the file's
/// basename once the status suffix is off.
#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub enum Source {
    /// The system journal (basename `system`, e.g. `system.journal`).
    System,
    /// A per-user journal; basename `user-<uid>` with the uid parsed as u32.
    User(u32),
    /// A journal forwarded from a remote host; basename `remote-<host>`.
    Remote(String),
    /// Any other basename, including a non-numeric `user-…`.
    Unknown(String),
}

impl Source {
    /// Classifies the last path component into a `Source` and returns the
    /// directory path above it; `None` when `path` has no directory
    /// component. Unmatched basenames become `Source::Unknown` instead of
    /// failing the parse.
    pub(super) fn parse(path: &str) -> Option<(Self, &str)> {
        // The basename is the component after the last '/'.
        let (dir_path, basename) = path.rsplit_once('/')?;

        let journal_type = if basename == "system" {
            Source::System
        } else if let Some(uid_str) = basename.strip_prefix("user-") {
            if let Ok(uid) = uid_str.parse::<u32>() {
                Source::User(uid)
            } else {
                Source::Unknown(basename.to_string())
            }
        } else if let Some(remote_host) = basename.strip_prefix("remote-") {
            Source::Remote(remote_host.to_string())
        } else {
            Source::Unknown(basename.to_string())
        };

        Some((journal_type, dir_path))
    }
}

/// Where a journal file comes from: machine, namespace and stream, parsed
/// from the path (module docs). The chains inside a repository directory
/// are keyed by this value (`Directory::chains` in repository/collection.rs).
#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct Origin {
    /// The machine the journal belongs to; `None` when there is no
    /// machine-id directory above the file, or when the directory's name
    /// is not a UUID (module docs).
    #[cfg_attr(feature = "allocative", allocative(skip))]
    pub machine_id: Option<Uuid>,
    /// Journal namespace (isolated journal instances); the part after `.`
    /// in the machine-id directory name.
    pub namespace: Option<String>,
    /// Stream classification from the basename (see `Source`).
    pub source: Source,
}

/// The `File` payload, `Arc`-shared behind `File::inner`; crate-internal.
#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub(crate) struct FileInner {
    pub(crate) path: String,
    pub(crate) origin: Origin,
    pub(crate) status: Status,
}

/// A parsed journal file path: the unit the repository data model works in.
///
/// Cloning is cheap (an `Arc` handle) and `Eq`/`Hash` cover path, origin and
/// status, so files work as map keys. Construct only through
/// `from_path`/`from_str`; the accepted naming scheme is in the module docs.
#[derive(Debug, Clone, PartialEq, Eq, Hash)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct File {
    pub(super) inner: Arc<FileInner>,
}

// Serialized form is the inner `{path, origin, status}` mapping (no Arc
// wrapper in the shape); deserializing rebuilds a fresh `Arc`.
impl serde::Serialize for File {
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        self.inner.as_ref().serialize(serializer)
    }
}

impl<'de> serde::Deserialize<'de> for File {
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        let inner = FileInner::deserialize(deserializer)?;
        Ok(File {
            inner: Arc::new(inner),
        })
    }
}

impl File {
    /// The stored path, exactly as parsed.
    pub fn path(&self) -> &str {
        &self.inner.path
    }

    /// Where the file comes from.
    pub fn origin(&self) -> &Origin {
        &self.inner.origin
    }

    /// Rotation status.
    pub fn status(&self) -> &Status {
        &self.inner.status
    }

    /// Parses a journal file path; `None` when the path is not valid UTF-8
    /// or does not match the naming scheme (see `from_str`).
    pub fn from_path(path: &Path) -> Option<Self> {
        Self::from_str(path.to_str()?)
    }

    /// Parses the naming scheme right to left into a `File`; see the module
    /// docs. Absolute paths only — the journal-log-writer rejects relative
    /// directories for exactly this reason
    /// (`journal-log-writer/src/log/mod.rs`). Every mismatch is a
    /// silent `None`; nothing here raises an error.
    //
    // Not `FromStr::from_str` — this parses a path; `from_path` is the
    // usual entry point.
    #[allow(clippy::should_implement_trait)]
    pub fn from_str(path: &str) -> Option<Self> {
        // Relative paths are rejected outright.
        if !path.starts_with("/") {
            return None;
        }

        // Right to left: status suffix, then source basename, then the
        // machine-id/namespace directory.
        let (status, path_after_status) = Status::parse(path)?;
        let (source, path_after_source) = Source::parse(path_after_status)?;

        // The machine-id/namespace directory is the basename's directory.
        let (machine_id, namespace) = if !path_after_source.is_empty() {
            // Its last component.
            let dirname = if let Some((_parent, dir)) = path_after_source.rsplit_once('/') {
                dir
            } else {
                path_after_source
            };

            if let Some((id_str, ns)) = dirname.split_once('.') {
                // `<machine-id>.<namespace>`: the machine-id half must parse
                // as a UUID or the file is rejected.
                let machine_id = Uuid::try_parse(id_str).ok()?;
                (Some(machine_id), Some(ns.to_string()))
            } else {
                // A plain dirname parses to a machine-id when it is a UUID;
                // otherwise the machine id stays unset but the file is kept.
                let machine_id = Uuid::try_parse(dirname).ok();
                (machine_id, None)
            }
        } else {
            (None, None)
        };

        let origin = Origin {
            machine_id,
            namespace,
            source,
        };

        let inner = Arc::new(FileInner {
            path: String::from(path),
            origin,
            status,
        });

        Some(File { inner })
    }

    /// The directory key the repository groups chains by: the parent of the
    /// file's directory when the origin's machine-id parsed (mapping
    /// `<base>/<machine-id>/…` up to `<base>`), else the file's directory
    /// itself. Fails only when the resolved directory is not valid UTF-8
    /// (`RepositoryError::InvalidUtf8`).
    pub fn dir(&self) -> Result<&str> {
        Path::new(&self.inner.path)
            .parent()
            .and_then(|p| {
                if self.inner.origin.machine_id.is_some() {
                    p.parent()
                } else {
                    Some(p)
                }
            })
            .and_then(|p| p.to_str())
            .ok_or_else(|| RepositoryError::InvalidUtf8 {
                path: Path::new(&self.inner.path).to_path_buf(),
            })
    }

    /// Extension-only check for journal paths (`.journal`/`.journal~`); the
    /// rest of the naming scheme is not validated.
    pub fn is_journal_file(path: &str) -> bool {
        path.ends_with(".journal") || path.ends_with(".journal~")
    }

    /// True for the live file (`Status::Active`).
    pub fn is_active(&self) -> bool {
        matches!(self.inner.status, Status::Active)
    }

    /// True for a rotated file (`Status::Archived`).
    pub fn is_archived(&self) -> bool {
        matches!(self.inner.status, Status::Archived { .. })
    }

    /// True for a corrupted/renamed file (`Status::Disposed`).
    pub fn is_disposed(&self) -> bool {
        matches!(self.inner.status, Status::Disposed { .. })
    }

    /// True for per-user journals (`Source::User`).
    pub fn is_user(&self) -> bool {
        matches!(self.inner.origin.source, Source::User(_))
    }

    /// True for the system journal (`Source::System`).
    pub fn is_system(&self) -> bool {
        matches!(self.inner.origin.source, Source::System)
    }

    /// True for remote-host journals (`Source::Remote`).
    pub fn is_remote(&self) -> bool {
        matches!(self.inner.origin.source, Source::Remote(_))
    }

    /// The uid of a per-user journal, else `None`.
    pub fn user_id(&self) -> Option<u32> {
        match &self.inner.origin.source {
            Source::User(uid) => Some(*uid),
            _ => None,
        }
    }

    /// The host of a remote journal, else `None`.
    pub fn remote_host(&self) -> Option<&str> {
        match &self.inner.origin.source {
            Source::Remote(host) => Some(host.as_str()),
            _ => None,
        }
    }

    /// The journal namespace, else `None`.
    pub fn namespace(&self) -> Option<&str> {
        self.inner.origin.namespace.as_deref()
    }
}

impl Ord for File {
    fn cmp(&self, other: &Self) -> Ordering {
        // Status first — the chain order — then the path, so equal-status
        // files get a deterministic slot for the binary search in
        // `Chain::insert_file` (repository/collection.rs).
        self.inner
            .status
            .cmp(&other.inner.status)
            .then_with(|| self.inner.path.cmp(&other.inner.path))
    }
}

impl PartialOrd for File {
    fn partial_cmp(&self, other: &Self) -> Option<Ordering> {
        Some(self.cmp(other))
    }
}

/// Recursively collects every journal file under `path` (naming scheme in
/// the module docs).
///
/// Walks with `walkdir` and does not descend into symlinked directories;
/// file entries whose path parses as a journal file (`File::from_path`)
/// are collected, everything else is skipped silently. Results come back in
/// walk order, unsorted — chains sort on insert
/// ([`crate::repository::Chain::insert_file`]). The walk is the only error
/// source: any
/// `walkdir::Error` (including a missing or unreadable root) becomes
/// `RepositoryError::WalkDir`, which is what makes the initial scan in
/// [`crate::Registry::watch_directory`] fallible — its only
/// workspace caller (`repository/mod.rs` re-exports the fn internally;
/// outside crates reach it by full path).
pub fn scan_journal_files(path: &str) -> Result<Vec<File>> {
    let mut files = Vec::new();

    for entry in walkdir::WalkDir::new(path).follow_links(false) {
        let entry = entry?;
        let path = entry.path();

        if path.is_file() {
            if let Some(file) = File::from_path(path) {
                files.push(file);
            }
        }
    }

    Ok(files)
}
