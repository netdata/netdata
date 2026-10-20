//! Tests for remote read-back, pinning the parent module's contract:
//! [`RemoteRead::fetch`] driven over the in-memory `MockStorage`, the
//! `download_deadline` formula and its enforcement on a paused tokio
//! clock, the `read_error_to_anyhow` redaction MUST, and
//! [`migrate_read_cache`]'s relocation of the logs-only cache to the shared
//! one.
//!
//! - `fetch` materializes entries in the download cache as
//!   `file_registry::SelectedFile`s: name = the `FileId`'s `.sfst`
//!   filename, content = the remote object's bytes, summary = the entry's
//!   stored one, `files` in request order, one pin per file.
//! - `progress` ticks once per download, completed or failed; a cache hit
//!   downloads nothing and does not tick, and entries naming the same file
//!   are fetched once.
//! - Per-object failure isolation: a failed, timed-out, or wrong-size object
//!   lands in [`RemoteFetch::failed`] and costs only itself — the remaining
//!   objects are still fetched, every object is attempted, and each download
//!   runs under its own size-derived deadline (30 s base + size at 1 MiB/s,
//!   capped at 5 min).
//! - The query-wide conditions surface as errors: a plan over the cache
//!   capacity is `CacheError::TooLarge`, a cancelled token
//!   `CacheError::Cancelled`.
//! - `read_error_to_anyhow` MUST flatten through `StorageError`'s redacted
//!   `Display`: the file-cache logs the anyhow chain verbatim with `{e:#}`,
//!   so a raw inner chain would put the STS web-identity JWT (carried in the
//!   URL query) into the journal.
//! - [`migrate_read_cache`]: an absent old cache is a no-op; plain
//!   directories rename `old` to `new`; `new` present or a failed rename
//!   removes the old cache's own files and torn writes, then the directory,
//!   leaving anything else; a directory symlink is re-pointed at its target
//!   (Unix), any other symlink only unlinked, a non-directory left; and a
//!   moved cache reopens with its surviving files.
//!
//! Not pinned here: `CacheError::EvictionFailed`, single-flight,
//! retry-on-vanish, and pin lifetime (file-cache's own tests); the
//! request-order placement of several failed entries (every test here fails
//! at most one); and [`RemoteRead::cache`], a plain accessor.
use super::*;
use crate::storage::{MockReadError, MockStorage};
use file_registry::{ByteSize, FileId, TimestampNs, test_identity};

const MIB: u64 = 1024 * 1024;

/// A catalog entry for seq `seq` whose object lives at `key-<seq>` and
/// declares `size` bytes.
fn entry(seq: u64, size: u64) -> CatalogEntry {
    CatalogEntry {
        id: FileId::new(test_identity(), 1, seq, 7),
        remote_key: format!("key-{seq}"),
        min_timestamp_s: 100 + seq as u32,
        max_timestamp_s: 200 + seq as u32,
        record_count: 6,
        content_meta: b"meta".to_vec(),
        size: ByteSize(size),
        uploaded_at_ns: TimestampNs(0),
        remote_etag: None,
    }
}

/// The cache filename `fetch` builds for `entry(seq)`'s file.
fn filename(seq: u64) -> String {
    FileId::new(test_identity(), 1, seq, 7).to_filename("sfst")
}

/// A mock whose objects `key-<seq>` hold `size` bytes each.
fn storage_with(objects: &[(u64, u64)]) -> MockStorage {
    let mut storage = MockStorage::default();
    for &(seq, size) in objects {
        storage
            .read_bodies
            .insert(format!("key-{seq}"), vec![seq as u8; size as usize]);
    }
    storage
}

/// A `RemoteRead` over `storage` with a fresh download cache of `capacity`.
/// The temp dir is `keep()`ed: a dropped `TempDir` would unlink the live
/// cache's directory.
fn remote(storage: MockStorage, capacity: u64) -> RemoteRead<MockStorage> {
    let cache = FileCache::open(tempfile::tempdir().unwrap().keep(), capacity).unwrap();
    RemoteRead::new(storage, cache)
}

/// Run a fetch and return it with the progress it ticked. `unwrap`s the
/// result, so the query-wide-`Err` test calls `remote.fetch` directly.
async fn fetch(
    remote: &RemoteRead<MockStorage>,
    entries: Vec<CatalogEntry>,
) -> (RemoteFetch, usize) {
    let progress = Arc::new(AtomicUsize::new(0));
    let fetched = remote
        .fetch(entries, &CancellationToken::new(), Arc::clone(&progress))
        .await
        .unwrap();
    (fetched, progress.load(Ordering::Relaxed))
}

fn fetched_seqs(fetched: &RemoteFetch) -> Vec<u64> {
    fetched.files.iter().map(|f| f.id.seq).collect()
}

fn failed_seqs(fetched: &RemoteFetch) -> Vec<u64> {
    fetched.failed.iter().map(|e| e.id.seq).collect()
}

#[test]
fn download_deadline_scales_with_size_and_is_capped() {
    let cases = [
        (0, 30),
        (1, 31),
        (MIB, 31),
        (MIB + 1, 32),
        (100 * MIB, 130),
        (270 * MIB, 300),
        (10 * 1024 * MIB, 300),
    ];
    for (size, secs) in cases {
        assert_eq!(
            download_deadline(size),
            Duration::from_secs(secs),
            "size {size}"
        );
    }
}

#[tokio::test]
async fn downloads_become_sealed_files_with_catalog_summaries() {
    let remote = remote(storage_with(&[(1, 10), (2, 20)]), MIB);
    let (fetched, progress) = fetch(&remote, vec![entry(1, 10), entry(2, 20)]).await;

    assert_eq!(fetched_seqs(&fetched), [1, 2]);
    assert!(fetched.failed.is_empty());
    assert_eq!(fetched.pins.len(), 2);
    assert_eq!(progress, 2, "one tick per download");
    let first = &fetched.files[0];
    assert_eq!(
        first.summary,
        sfst::Summary {
            min_timestamp_s: 101,
            max_timestamp_s: 201,
            record_count: 6,
            content_meta: b"meta".to_vec(),
        }
    );
    assert_eq!(
        first.path.file_name().unwrap().to_str().unwrap(),
        filename(1)
    );
    assert_eq!(std::fs::read(&first.path).unwrap(), vec![1u8; 10]);
}

#[tokio::test]
async fn cached_files_are_not_downloaded_again() {
    let storage = storage_with(&[(1, 10)]);
    let reads = Arc::clone(&storage.read_calls);
    let remote = remote(storage, MIB);
    let (first, _) = fetch(&remote, vec![entry(1, 10)]).await;
    drop(first);
    let (second, progress) = fetch(&remote, vec![entry(1, 10)]).await;

    assert_eq!(fetched_seqs(&second), [1]);
    assert_eq!(reads.load(Ordering::Relaxed), 1);
    assert_eq!(progress, 0, "a cache hit downloads nothing");
}

#[tokio::test]
async fn duplicate_entries_are_fetched_once() {
    let storage = storage_with(&[(1, 10)]);
    let reads = Arc::clone(&storage.read_calls);
    let remote = remote(storage, MIB);
    let (fetched, _) = fetch(&remote, vec![entry(1, 10), entry(1, 10)]).await;

    assert_eq!(fetched_seqs(&fetched), [1]);
    assert!(fetched.failed.is_empty());
    assert_eq!(reads.load(Ordering::Relaxed), 1);
}

#[tokio::test]
async fn a_failed_download_costs_only_its_own_file() {
    for (label, error) in [
        ("not found", MockReadError::NotFound),
        ("storage error", MockReadError::Other),
    ] {
        let mut storage = storage_with(&[(2, 10), (3, 10)]);
        storage.read_key_errors.insert("key-1".to_string(), error);
        let reads = Arc::clone(&storage.read_calls);
        let remote = remote(storage, MIB);
        let (fetched, progress) =
            fetch(&remote, vec![entry(1, 10), entry(2, 10), entry(3, 10)]).await;

        assert_eq!(fetched_seqs(&fetched), [2, 3], "{label}");
        assert_eq!(failed_seqs(&fetched), [1], "{label}");
        assert_eq!(
            reads.load(Ordering::Relaxed),
            3,
            "{label}: every object is read"
        );
        assert_eq!(progress, 3, "{label}: failures tick too");
    }
}

#[tokio::test]
async fn a_wrong_size_object_is_failed() {
    let remote = remote(storage_with(&[(1, 9)]), MIB);
    let (fetched, _) = fetch(&remote, vec![entry(1, 10)]).await;

    assert!(fetched.files.is_empty());
    assert_eq!(failed_seqs(&fetched), [1]);
}

#[tokio::test(start_paused = true)]
async fn a_timed_out_download_costs_only_its_own_file() {
    let mut storage = storage_with(&[(1, 10), (2, 10)]);
    storage
        .read_delays
        .insert("key-1".to_string(), Duration::from_secs(40));
    let reads = Arc::clone(&storage.read_calls);
    let remote = remote(storage, MIB);
    let started = tokio::time::Instant::now();
    let (fetched, progress) = fetch(&remote, vec![entry(1, 10), entry(2, 10)]).await;

    assert_eq!(fetched_seqs(&fetched), [2]);
    assert_eq!(failed_seqs(&fetched), [1]);
    assert_eq!(
        started.elapsed(),
        Duration::from_secs(31),
        "the 10-byte deadline, then the next download"
    );
    assert_eq!(reads.load(Ordering::Relaxed), 2, "every object is read");
    assert_eq!(progress, 2);
}

#[tokio::test(start_paused = true)]
async fn a_slow_download_within_its_size_allowance_succeeds() {
    // 2 MiB may take 32 s: slower than the 30 s base, still in time.
    let mut storage = storage_with(&[(1, 2 * MIB)]);
    storage
        .read_delays
        .insert("key-1".to_string(), Duration::from_millis(31_500));
    let remote = remote(storage, 4 * MIB);
    let (fetched, _) = fetch(&remote, vec![entry(1, 2 * MIB)]).await;

    assert_eq!(fetched_seqs(&fetched), [1]);
    assert!(fetched.failed.is_empty());
}

#[tokio::test(start_paused = true)]
async fn no_download_waits_longer_than_five_minutes() {
    // 10 GiB would earn 10,270 s at the minimum rate; the cap stops it at 300.
    let size = 10 * 1024 * MIB;
    let mut storage = MockStorage::default();
    storage
        .read_delays
        .insert("key-1".to_string(), Duration::from_secs(10_000));
    let remote = remote(storage, 2 * size);
    let started = tokio::time::Instant::now();
    let (fetched, _) = fetch(&remote, vec![entry(1, size)]).await;

    assert_eq!(failed_seqs(&fetched), [1]);
    assert_eq!(started.elapsed(), Duration::from_secs(300));
}

#[tokio::test]
async fn query_wide_conditions_are_errors() {
    let remote = remote(storage_with(&[(1, 10), (2, 10)]), 15);
    let too_large = remote
        .fetch(
            vec![entry(1, 10), entry(2, 10)],
            &CancellationToken::new(),
            Arc::new(AtomicUsize::new(0)),
        )
        .await;
    assert!(matches!(
        too_large,
        Err(CacheError::TooLarge {
            footprint: 20,
            capacity: 15
        })
    ));

    let cancel = CancellationToken::new();
    cancel.cancel();
    let cancelled = remote
        .fetch(vec![entry(1, 10)], &cancel, Arc::new(AtomicUsize::new(0)))
        .await;
    assert!(matches!(cancelled, Err(CacheError::Cancelled)));
}

#[test]
fn read_errors_reach_the_cache_log_redacted() {
    // The cache logs this error with `{e:#}` — a raw inner chain would put
    // the STS token (carried in the URL query) into the journal. Pin that
    // the conversion flattens through StorageError's redacted Display.
    let inner = anyhow::anyhow!(
        "error sending request for url (https://sts.amazonaws.com/?Action=AssumeRoleWithWebIdentity&WebIdentityToken=SENTINEL_JWT)"
    );
    let err = read_error_to_anyhow("v2/logs/x.sfst", StorageError::Other(inner));
    let rendered = format!("{err:#}");
    assert!(!rendered.contains("SENTINEL_JWT"), "leaked: {rendered}");
    assert!(
        rendered.contains("remote read failed for v2/logs/x.sfst"),
        "context lost: {rendered}"
    );
    assert!(
        rendered.contains("https://sts.amazonaws.com/?[REDACTED]"),
        "cause lost or unredacted: {rendered}"
    );

    let nf = read_error_to_anyhow("v2/logs/x.sfst", StorageError::NotFound);
    assert!(format!("{nf:#}").contains("remote object not found"));
}

// ── migrate_read_cache ──────────────────────────────────────────────

/// A base dir with the old (`logs/remote-read`) and new (`remote-read`)
/// cache locations; neither exists yet.
struct Layout {
    _base: tempfile::TempDir,
    old: PathBuf,
    new: PathBuf,
}

fn layout() -> Layout {
    let base = tempfile::tempdir().unwrap();
    std::fs::create_dir_all(base.path().join("logs")).unwrap();
    Layout {
        old: base.path().join("logs").join("remote-read"),
        new: base.path().join("remote-read"),
        _base: base,
    }
}

/// A directory holding `files` (name, bytes).
fn dir_with(dir: &Path, files: &[(&str, &[u8])]) {
    std::fs::create_dir_all(dir).unwrap();
    for (name, bytes) in files {
        std::fs::write(dir.join(name), bytes).unwrap();
    }
}

/// Sorted names directly inside `dir`.
fn names_in(dir: &Path) -> Vec<String> {
    let mut names: Vec<String> = std::fs::read_dir(dir)
        .unwrap()
        .map(|e| e.unwrap().file_name().into_string().unwrap())
        .collect();
    names.sort();
    names
}

/// Whether `path` exists, counting a dangling symlink (`Path::exists` would
/// follow the link and miss it).
fn exists(path: &Path) -> bool {
    std::fs::symlink_metadata(path).is_ok()
}

#[test]
fn the_old_cache_is_moved_to_the_new_location() {
    let l = layout();
    dir_with(&l.old, &[(&filename(1), b"one")]);

    migrate_read_cache(&l.old, &l.new);

    assert!(!exists(&l.old));
    assert_eq!(names_in(&l.new), [filename(1)]);
    assert_eq!(std::fs::read(l.new.join(filename(1))).unwrap(), b"one");
}

#[test]
fn an_absent_old_cache_changes_nothing() {
    let l = layout();

    migrate_read_cache(&l.old, &l.new);

    assert!(!exists(&l.old));
    assert!(!exists(&l.new));
}

#[test]
fn an_old_cache_beside_a_new_one_is_emptied_and_removed() {
    let l = layout();
    dir_with(&l.old, &[(&filename(1), b"old"), ("x.sfst.tmp", b"torn")]);
    dir_with(&l.new, &[(&filename(2), b"new")]);

    migrate_read_cache(&l.old, &l.new);

    assert!(!exists(&l.old));
    assert_eq!(names_in(&l.new), [filename(2)]);
}

#[test]
fn a_failed_move_removes_the_old_cache() {
    let l = layout();
    dir_with(&l.old, &[(&filename(1), b"old")]);

    migrate_read_cache_with(&l.old, &l.new, |_, _| {
        Err(io::Error::from(io::ErrorKind::CrossesDevices))
    });

    assert!(!exists(&l.old));
    assert!(!exists(&l.new), "the cache creates it when it opens");
}

#[test]
fn files_the_cache_did_not_write_are_kept() {
    let l = layout();
    dir_with(
        &l.old,
        &[
            (&filename(1), b"old"),
            ("notes.txt", b"operator"),
            ("y.tmp", b"torn"),
        ],
    );
    std::fs::create_dir(l.old.join("sub")).unwrap();
    // `new` exists, so the rename path is skipped: `old`'s cache files are
    // removed in place.
    dir_with(&l.new, &[]);

    migrate_read_cache(&l.old, &l.new);

    assert_eq!(names_in(&l.old), ["notes.txt", "sub"]);
}

#[cfg(unix)]
#[test]
fn a_linked_old_cache_keeps_pointing_at_its_directory() {
    let l = layout();
    let target = tempfile::tempdir().unwrap();
    dir_with(target.path(), &[(&filename(1), b"one")]);
    // A relative link: moving it as-is would change what it points to.
    let relative = pathdiff(target.path(), l.old.parent().unwrap());
    std::os::unix::fs::symlink(&relative, &l.old).unwrap();

    migrate_read_cache(&l.old, &l.new);

    assert!(!exists(&l.old));
    assert_eq!(
        std::fs::read_link(&l.new).unwrap(),
        std::fs::canonicalize(target.path()).unwrap()
    );
    assert_eq!(names_in(&l.new), [filename(1)]);
}

#[cfg(unix)]
#[test]
fn any_other_old_link_is_removed_alone() {
    // Beside an existing new cache.
    let l = layout();
    let target = tempfile::tempdir().unwrap();
    dir_with(target.path(), &[(&filename(1), b"one")]);
    std::os::unix::fs::symlink(target.path(), &l.old).unwrap();
    dir_with(&l.new, &[(&filename(2), b"new")]);

    migrate_read_cache(&l.old, &l.new);

    assert!(!exists(&l.old));
    assert_eq!(
        names_in(target.path()),
        [filename(1)],
        "the target is untouched"
    );
    assert_eq!(names_in(&l.new), [filename(2)]);

    // Dangling.
    let l = layout();
    std::os::unix::fs::symlink(l.old.parent().unwrap().join("gone"), &l.old).unwrap();

    migrate_read_cache(&l.old, &l.new);

    assert!(!exists(&l.old));
    assert!(!exists(&l.new));
}

#[test]
fn an_old_cache_that_is_not_a_directory_is_left() {
    let l = layout();
    std::fs::write(&l.old, b"not a directory").unwrap();

    migrate_read_cache(&l.old, &l.new);

    assert_eq!(std::fs::read(&l.old).unwrap(), b"not a directory");
    assert!(!exists(&l.new));
}

#[test]
fn a_moved_cache_reopens_with_its_files_and_without_torn_writes() {
    let l = layout();
    dir_with(&l.old, &[(&filename(1), b"one"), ("z.tmp", b"torn")]);

    migrate_read_cache(&l.old, &l.new);
    // `z.tmp` rode along the rename; the sweep on `open` removes it.
    let cache = FileCache::open(&l.new, MIB).unwrap();

    assert!(cache.is_cached(&filename(1)));
    assert_eq!(names_in(&l.new), [filename(1)]);
}

/// `path` relative to `base` (both absolute; `base` has no symlinks).
#[cfg(unix)]
fn pathdiff(path: &Path, base: &Path) -> PathBuf {
    let path = std::fs::canonicalize(path).unwrap();
    let base = std::fs::canonicalize(base).unwrap();
    let common = path
        .components()
        .zip(base.components())
        .take_while(|(a, b)| a == b)
        .count();
    let mut out = PathBuf::new();
    for _ in base.components().skip(common) {
        out.push("..");
    }
    for part in path.components().skip(common) {
        out.push(part);
    }
    out
}
