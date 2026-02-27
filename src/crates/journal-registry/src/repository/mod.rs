//! Journal file repository
//!
//! This module provides types and data structures for representing and organizing
//! systemd journal files into a queryable repository.
//!
//! ## Key Components
//!
//! - **File**: Represents a journal file with parsed metadata (origin, status)
//! - **Origin**: Identifies where a journal file comes from (system, user, remote)
//! - **Status**: Indicates whether a file is active, archived, or disposed
//! - **Chain**: An ordered collection of journal files from the same origin
//! - **Repository**: The top-level container organizing chains by directory and origin
//! - **FileInfo**: Associates a file with its time range metadata
//! - **TimeRange**: Tracks the temporal bounds of indexed journal files
//!
//! ## Architecture
//!
//! The repository organizes files in a three-level hierarchy:
//! ```text
//! Repository
//!   └─ Directory (/var/log/journal)
//!       └─ Origin (System, User(1000), Remote("host"))
//!           └─ Chain (ordered list of files)
//! ```
//!
//! Files within a chain are kept sorted:
//! - Disposed files (corrupted) come first
//! - Archived files follow in chronological order
//! - Active file (if any) comes last

// Public modules - accessible to workspace crates via full paths
pub mod collection;
pub mod error;
pub mod file;
pub mod metadata;

// Re-export only the public API types
pub use crate::repository::file::{File, Origin, Source, Status};
pub use crate::repository::metadata::FileInfo;

// Re-export workspace-internal types (hidden from public docs)
// These are not in lib.rs exports but accessible via full paths for workspace crates
#[doc(hidden)]
pub use crate::repository::collection::{Chain, Repository};
#[doc(hidden)]
pub use crate::repository::error::RepositoryError;

// Crate-internal only
pub(crate) use crate::repository::file::scan_journal_files;

#[cfg(test)]
mod tests {
    use super::*;
    use crate::repository::collection::Chain;
    use crate::repository::file::FileInner;
    use journal_common::Seconds;
    use journal_common::collections::VecDeque;
    use std::sync::Arc;
    use uuid::Uuid;

    const USEC_PER_SEC: u64 = std::time::Duration::from_secs(1).as_micros() as u64;

    fn create_test_origin() -> Origin {
        Origin {
            machine_id: Some(Uuid::new_v4()),
            namespace: None,
            source: Source::System,
        }
    }

    fn create_archived_file(origin: &Origin, head_realtime: u64) -> File {
        let inner = FileInner {
            path: format!("/var/log/journal/system@{}.journal", head_realtime),
            origin: origin.clone(),
            status: Status::Archived {
                seqnum_id: Uuid::new_v4(),
                head_seqnum: 1000 + head_realtime,
                head_realtime,
            },
        };

        File {
            inner: Arc::new(inner),
        }
    }

    fn create_active_file(origin: &Origin) -> File {
        let inner = FileInner {
            path: "/var/log/journal/system.journal".to_string(),
            origin: origin.clone(),
            status: Status::Active,
        };

        File {
            inner: Arc::new(inner),
        }
    }

    fn create_disposed_file(origin: &Origin, timestamp: u64, number: u64) -> File {
        let inner = FileInner {
            path: format!("/var/log/journal/system@{}-{}.journal~", timestamp, number),
            origin: origin.clone(),
            status: Status::Disposed { timestamp, number },
        };

        File {
            inner: Arc::new(inner),
        }
    }

    #[test]
    fn test_find_files_in_range_empty_chain() {
        let chain = Chain {
            files: VecDeque::new(),
        };

        let mut files = Vec::new();
        chain.find_files_in_range(Seconds(100), Seconds(200), &mut files);
        assert!(files.is_empty());
    }

    #[test]
    fn test_find_files_in_range_invalid_range() {
        let origin = create_test_origin();
        let mut chain = Chain {
            files: VecDeque::new(),
        };

        // Add some files
        chain
            .files
            .push_back(create_archived_file(&origin, 100 * USEC_PER_SEC));
        chain
            .files
            .push_back(create_archived_file(&origin, 200 * USEC_PER_SEC));

        let mut files = Vec::new();
        // Test with start >= end
        chain.find_files_in_range(Seconds(200), Seconds(200), &mut files);
        assert!(files.is_empty());

        chain.find_files_in_range(Seconds(200), Seconds(100), &mut files);
        assert!(files.is_empty());
    }

    #[test]
    fn test_find_files_in_range_single_archived() {
        let origin = create_test_origin();
        let mut chain = Chain {
            files: VecDeque::new(),
        };

        // Single archived file with head_realtime = 150
        let file = create_archived_file(&origin, 150 * USEC_PER_SEC);
        chain.files.push_back(file.clone());

        // Test range that starts before and ends after the file
        let mut files = Vec::new();
        chain.find_files_in_range(Seconds(100), Seconds(200), &mut files);
        assert_eq!(files.len(), 1);
        assert_eq!(files[0], file);

        // Test range that starts exactly at head_realtime
        files.clear();
        chain.find_files_in_range(Seconds(150), Seconds(200), &mut files);
        assert_eq!(files.len(), 1);
        assert_eq!(files[0], file);

        // Test range that ends exactly at head_realtime (should not include)
        files.clear();
        chain.find_files_in_range(Seconds(100), Seconds(150), &mut files);
        assert!(files.is_empty());

        // Test range entirely before the file
        files.clear();
        chain.find_files_in_range(Seconds(50), Seconds(100), &mut files);
        assert!(files.is_empty());

        // Test range entirely after (single archived file extends to infinity)
        files.clear();
        chain.find_files_in_range(Seconds(200), Seconds(300), &mut files);
        assert_eq!(files.len(), 1);
        assert_eq!(files[0], file);
    }

    #[test]
    fn test_find_files_in_range_multiple_archived() {
        let origin = create_test_origin();
        let mut chain = Chain {
            files: VecDeque::new(),
        };

        // Multiple archived files: 100, 200, 300, 400
        let file1 = create_archived_file(&origin, 100 * USEC_PER_SEC);
        let file2 = create_archived_file(&origin, 200 * USEC_PER_SEC);
        let file3 = create_archived_file(&origin, 300 * USEC_PER_SEC);
        let file4 = create_archived_file(&origin, 400 * USEC_PER_SEC);

        chain.files.push_back(file1.clone());
        chain.files.push_back(file2.clone());
        chain.files.push_back(file3.clone());
        chain.files.push_back(file4.clone());

        // Range [150, 350) should include files at 100, 200, and 300 in order
        let mut files = Vec::new();
        chain.find_files_in_range(Seconds(150), Seconds(350), &mut files);
        assert_eq!(files.len(), 3);
        assert_eq!(files[0], file1); // Files returned in chronological order
        assert_eq!(files[1], file2);
        assert_eq!(files[2], file3);

        // Range [200, 300) should include only file at 200
        files.clear();
        chain.find_files_in_range(Seconds(200), Seconds(300), &mut files);
        assert_eq!(files.len(), 1);
        assert_eq!(files[0], file2);

        // Range [250, 350) should include files at 200 and 300 in order
        files.clear();
        chain.find_files_in_range(Seconds(250), Seconds(350), &mut files);
        assert_eq!(files.len(), 2);
        assert_eq!(files[0], file2);
        assert_eq!(files[1], file3);

        // Range [450, 500) should include file at 400 (last file extends to infinity)
        files.clear();
        chain.find_files_in_range(Seconds(450), Seconds(500), &mut files);
        assert_eq!(files.len(), 1);
        assert_eq!(files[0], file4);
    }

    #[test]
    fn test_find_files_in_range_with_active() {
        let origin = create_test_origin();
        let mut chain = Chain {
            files: VecDeque::new(),
        };

        // Archived files at 100, 200, then active
        let file1 = create_archived_file(&origin, 100 * USEC_PER_SEC);
        let file2 = create_archived_file(&origin, 200 * USEC_PER_SEC);
        let active = create_active_file(&origin);

        chain.files.push_back(file1.clone());
        chain.files.push_back(file2.clone());
        chain.files.push_back(active.clone());

        // Range [150, 250) should include files at 100, 200, and active in order
        let mut files = Vec::new();
        chain.find_files_in_range(Seconds(150), Seconds(250), &mut files);
        assert_eq!(files.len(), 3);
        assert_eq!(files[0], file1); // Archived files in chronological order
        assert_eq!(files[1], file2);
        assert_eq!(files[2], active); // Active file comes last

        // Range [250, 350) should include only file at 200 and active in order
        files.clear();
        chain.find_files_in_range(Seconds(250), Seconds(350), &mut files);
        assert_eq!(files.len(), 2);
        assert_eq!(files[0], file2);
        assert_eq!(files[1], active);

        // Range [50, 150) should include file at 100
        files.clear();
        chain.find_files_in_range(Seconds(50), Seconds(150), &mut files);
        assert_eq!(files.len(), 1);
        assert_eq!(files[0], file1);
    }

    #[test]
    fn test_find_files_in_range_only_active() {
        let origin = create_test_origin();
        let mut chain = Chain {
            files: VecDeque::new(),
        };

        let active = create_active_file(&origin);
        chain.files.push_back(active.clone());

        // Active file with no archived files should span from u64::MIN to u64::MAX
        let mut files = Vec::new();
        chain.find_files_in_range(Seconds(0), Seconds(100), &mut files);
        assert_eq!(files.len(), 1);
        assert_eq!(files[0], active);

        files.clear();
        let start = Seconds(u32::MAX - 100);
        let end = Seconds(u32::MAX);
        chain.find_files_in_range(start, end, &mut files);
        assert_eq!(files.len(), 1);
        assert_eq!(files[0], active);
    }

    #[test]
    fn test_find_files_in_range_with_disposed() {
        let origin = create_test_origin();
        let mut chain = Chain {
            files: VecDeque::new(),
        };

        // Disposed files should be at the beginning and should be skipped
        let disposed1 = create_disposed_file(&origin, 50 * USEC_PER_SEC, 1);
        let disposed2 = create_disposed_file(&origin, 60 * USEC_PER_SEC, 2);
        let file1 = create_archived_file(&origin, 100 * USEC_PER_SEC);
        let file2 = create_archived_file(&origin, 200 * USEC_PER_SEC);

        chain.files.push_back(disposed1);
        chain.files.push_back(disposed2);
        chain.files.push_back(file1.clone());
        chain.files.push_back(file2.clone());

        // Disposed files should not appear in output, only archived files in order
        let mut files = Vec::new();
        chain.find_files_in_range(Seconds(0), Seconds(300), &mut files);
        assert_eq!(files.len(), 2);
        assert_eq!(files[0], file1); // Files in chronological order
        assert_eq!(files[1], file2);
    }

    #[test]
    fn test_find_files_in_range_edge_cases() {
        let origin = create_test_origin();
        let mut chain = Chain {
            files: VecDeque::new(),
        };

        // Files at 100, 200, 300
        let file1 = create_archived_file(&origin, 100 * USEC_PER_SEC);
        let file2 = create_archived_file(&origin, 200 * USEC_PER_SEC);
        let file3 = create_archived_file(&origin, 300 * USEC_PER_SEC);

        chain.files.push_back(file1.clone());
        chain.files.push_back(file2.clone());
        chain.files.push_back(file3.clone());

        // Test exact boundaries
        let mut files = Vec::new();

        // Range [100, 200) should include only file at 100
        chain.find_files_in_range(Seconds(100), Seconds(200), &mut files);
        assert_eq!(files.len(), 1);
        assert_eq!(files[0], file1);

        // Range [200, 300) should include only file at 200
        files.clear();
        chain.find_files_in_range(Seconds(200), Seconds(300), &mut files);
        assert_eq!(files.len(), 1);
        assert_eq!(files[0], file2);

        // Range [300, 400) should include only file at 300
        files.clear();
        chain.find_files_in_range(Seconds(300), Seconds(400), &mut files);
        assert_eq!(files.len(), 1);
        assert_eq!(files[0], file3);

        // Range [199, 201) should include files at 100 and 200 in order
        files.clear();
        chain.find_files_in_range(Seconds(199), Seconds(201), &mut files);
        assert_eq!(files.len(), 2);
        assert_eq!(files[0], file1);
        assert_eq!(files[1], file2);
    }

    #[test]
    fn test_find_files_in_range_complex_scenario() {
        let origin = create_test_origin();
        let mut chain = Chain {
            files: VecDeque::new(),
        };

        // Complex scenario with disposed, archived, and active files
        let disposed = create_disposed_file(&origin, 10 * USEC_PER_SEC, 1);
        let file1 = create_archived_file(&origin, 1000 * USEC_PER_SEC);
        let file2 = create_archived_file(&origin, 2000 * USEC_PER_SEC);
        let file3 = create_archived_file(&origin, 3000 * USEC_PER_SEC);
        let file4 = create_archived_file(&origin, 4000 * USEC_PER_SEC);
        let active = create_active_file(&origin);

        chain.files.push_back(disposed);
        chain.files.push_back(file1.clone());
        chain.files.push_back(file2.clone());
        chain.files.push_back(file3.clone());
        chain.files.push_back(file4.clone());
        chain.files.push_back(active.clone());

        // Range [1500, 3500) should include files at 1000, 2000, 3000 in chronological order
        let mut files = Vec::new();
        chain.find_files_in_range(Seconds(1500), Seconds(3500), &mut files);
        assert_eq!(files.len(), 3);
        assert_eq!(files[0], file1); // Files returned in chronological order
        assert_eq!(files[1], file2);
        assert_eq!(files[2], file3);

        // Range [4500, 5000) should include file4 and active in order
        files.clear();
        chain.find_files_in_range(Seconds(4500), Seconds(5000), &mut files);
        assert_eq!(files.len(), 2);
        assert_eq!(files[0], file4); // Last archived file
        assert_eq!(files[1], active); // Active file comes last

        // Range [500, 1500) should include file at 1000
        files.clear();
        chain.find_files_in_range(Seconds(500), Seconds(1500), &mut files);
        assert_eq!(files.len(), 1);
        assert_eq!(files[0], file1);

        // Range covering everything - all files in chronological order
        files.clear();
        chain.find_files_in_range(Seconds(0), Seconds(u32::MAX), &mut files);
        assert_eq!(files.len(), 5); // All except disposed
        assert_eq!(files[0], file1); // Chronological order: 1000, 2000, 3000, 4000, active
        assert_eq!(files[1], file2);
        assert_eq!(files[2], file3);
        assert_eq!(files[3], file4);
        assert_eq!(files[4], active);
    }
}
