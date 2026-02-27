use crate::repository::RepositoryError;
use crate::repository::error::Result;
use serde::{Deserialize, Serialize};
use std::cmp::Ordering;
use std::path::Path;
use std::sync::Arc;
use uuid::Uuid;

/// Status of a journal file
#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub enum Status {
    /// Active journal file currently being written to
    Active,
    /// Archived journal file that has been rotated and is no longer being written to
    Archived {
        /// Sequence number ID for ordering entries across files
        #[cfg_attr(feature = "allocative", allocative(skip))]
        seqnum_id: Uuid,
        /// Sequence number of the first entry in this file
        head_seqnum: u64,
        /// Realtime timestamp (microseconds since epoch) of the first entry
        head_realtime: u64,
    },
    /// Disposed (corrupted or incomplete) journal file marked for cleanup
    Disposed {
        /// Timestamp when the file was disposed (microseconds since epoch)
        timestamp: u64,
        /// Sequence number for ordering multiple disposed files
        number: u64,
    },
}

impl Ord for Status {
    fn cmp(&self, other: &Self) -> Ordering {
        match (self, other) {
            // Disposed files come first, sorted by timestamp then number
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

            // Disposed always comes before non-disposed
            (Status::Disposed { .. }, _) => Ordering::Less,
            (_, Status::Disposed { .. }) => Ordering::Greater,

            // Archived files sorted by head_realtime (then seqnum for stability)
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

            // Archived comes before Active
            (Status::Archived { .. }, Status::Active) => Ordering::Less,
            (Status::Active, Status::Archived { .. }) => Ordering::Greater,

            // Active files are equal in terms of status ordering
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
    /// Parse the journal file status from the end of the path, returning the status and the remaining path
    pub(super) fn parse(path: &str) -> Option<(Self, &str)> {
        if let Some(stem) = path.strip_suffix(".journal") {
            // Check if it's archived (has @ suffix) or active
            if let Some((prefix, suffix)) = stem.rsplit_once('@') {
                // Parse archived format: @seqnum_id-head_seqnum-head_realtime
                let mut parts = suffix.split('-');

                let seqnum_id = parts.next()?;
                let head_seqnum = parts.next()?;
                let head_realtime = parts.next()?;

                if parts.next().is_some() {
                    return None; // Too many parts
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
                // Active journal
                Some((Status::Active, stem))
            }
        } else if let Some(stem) = path.strip_suffix(".journal~") {
            // Disposed format: @timestamp-number.journal~
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

/// Source of journal entries
#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub enum Source {
    /// System-wide journal (system.journal)
    System,
    /// User-specific journal with the given UID
    User(u32),
    /// Journal from a remote host
    Remote(String),
    /// Unknown or non-standard journal type
    Unknown(String),
}

impl Source {
    /// Parse the journal basename from the end of the path, returning the basename and the remaining path
    pub(super) fn parse(path: &str) -> Option<(Self, &str)> {
        // Split on the last '/' to get directory and basename
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

/// Origin identifies where a journal file comes from
#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct Origin {
    /// Machine ID from which the journal originates
    #[cfg_attr(feature = "allocative", allocative(skip))]
    pub machine_id: Option<Uuid>,
    /// Optional namespace for isolated journal instances
    pub namespace: Option<String>,
    /// Source type (system, user, remote, or unknown)
    pub source: Source,
}

#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub(crate) struct FileInner {
    pub(crate) path: String,
    pub(crate) origin: Origin,
    pub(crate) status: Status,
}

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct File {
    pub(super) inner: Arc<FileInner>,
}

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
    pub fn path(&self) -> &str {
        &self.inner.path
    }

    pub fn origin(&self) -> &Origin {
        &self.inner.origin
    }

    pub fn status(&self) -> &Status {
        &self.inner.status
    }

    pub fn from_path(path: &Path) -> Option<Self> {
        Self::from_str(path.to_str()?)
    }

    #[allow(clippy::should_implement_trait)]
    pub fn from_str(path: &str) -> Option<Self> {
        // We only accept absolute paths
        if !path.starts_with("/") {
            return None;
        }

        // Parse from right to left
        let (status, path_after_status) = Status::parse(path)?;
        let (source, path_after_source) = Source::parse(path_after_status)?;

        // Try to parse machine ID and namespace from the directory name
        let (machine_id, namespace) = if !path_after_source.is_empty() {
            // Get the last directory component
            let dirname = if let Some((_parent, dir)) = path_after_source.rsplit_once('/') {
                dir
            } else {
                path_after_source
            };

            if let Some((id_str, ns)) = dirname.split_once('.') {
                // Has namespace
                let machine_id = Uuid::try_parse(id_str).ok()?;
                (Some(machine_id), Some(ns.to_string()))
            } else {
                // No namespace, just machine ID
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

    /// Check if a path looks like a journal file
    pub fn is_journal_file(path: &str) -> bool {
        path.ends_with(".journal") || path.ends_with(".journal~")
    }

    /// Check if this is an active journal file that's currently being written to
    pub fn is_active(&self) -> bool {
        matches!(self.inner.status, Status::Active)
    }

    /// Check if this is an archived journal file
    pub fn is_archived(&self) -> bool {
        matches!(self.inner.status, Status::Archived { .. })
    }

    /// Check if this is a corrupted/disposed journal file
    pub fn is_disposed(&self) -> bool {
        matches!(self.inner.status, Status::Disposed { .. })
    }

    /// Check if this contains logs from users
    pub fn is_user(&self) -> bool {
        matches!(self.inner.origin.source, Source::User(_))
    }

    /// Check if this contains logs from system
    pub fn is_system(&self) -> bool {
        matches!(self.inner.origin.source, Source::System)
    }

    pub fn is_remote(&self) -> bool {
        matches!(self.inner.origin.source, Source::Remote(_))
    }

    /// Get the user ID if this is a user journal
    pub fn user_id(&self) -> Option<u32> {
        match &self.inner.origin.source {
            Source::User(uid) => Some(*uid),
            _ => None,
        }
    }

    /// Get the remote host if this is a remote journal
    pub fn remote_host(&self) -> Option<&str> {
        match &self.inner.origin.source {
            Source::Remote(host) => Some(host.as_str()),
            _ => None,
        }
    }

    /// Get the namespace if this journal belongs to a namespace
    pub fn namespace(&self) -> Option<&str> {
        self.inner.origin.namespace.as_deref()
    }
}

impl Ord for File {
    fn cmp(&self, other: &Self) -> Ordering {
        // First compare by status, then by path for stability
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

/// Scan a directory recursively for journal files
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
