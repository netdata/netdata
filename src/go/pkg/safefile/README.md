# Safe file operations

This package has two distinct contracts. Configured inputs may use symlinks; collector-owned state must reject them.
Neither contract changes process authority. Use `pkg/credentialfile` for configured credential reads.

| API | Contract |
| --- | --- |
| `Open`, `Read` | Follow symlinks, then verify the opened object is regular. `Read` accepts at most `MaxSize` (1 MiB). |
| `ReadNoFollow(path, limit)` | Refuse symlink/special file leaves and a symlinked immediate parent. Verify the opened descriptor and enforce the caller's byte limit, including growth during reading. |
| `Replace(path, data, mode)` | Verify the parent and existing destination leaf; exclusively create a randomized sibling, write, fsync, close and rename it. Never truncate an existing temp or sweep foreign temps. |
| `EnsureDir(path, mode)` | Create missing directories and verify the final directory without following its leaf symlink; exact permissions normally use its descriptor. |
| `SyncDir(path)` | Verify and fsync the directory; Windows verifies but skips fsync. |

Parent checks for `ReadNoFollow` and `Replace` inspect directory metadata without opening it for reading. Named-child
access requires search permission; replacement also requires write permission. Neither operation changes directory
permissions. Directory chmod and fsync use verified handles. This separation keeps read-only startup able to inspect
ownership before active takeover repairs permissions, including when the state file is absent.

`Mode{Perm: 0640}` respects umask on creation and preserves existing directory permissions.
`Mode{Perm: 0600, Exact: true}` applies that mode through the file descriptor, overriding umask.
When an owned Unix directory cannot be opened for reading, exact mode uses no-follow chmod relative to a verified
parent, then reopens and verifies the directory. A concurrent replacement leaf can itself be chmodded by that fallback;
its symlink target is never followed. Linux kernels lacking fchmodat2 require `/proc/self/fd` for this fallback.
Windows permission semantics remain those of `os.File.Chmod`, not Unix access-control semantics.

`Replace` deliberately stops at rename. A successful call means the replacement is visible; callers MUST then call
`SyncDir` before claiming crash durability. A subsequent directory-sync error cannot be treated as an unpublished
write: S3 journals latch that uncertainty and require a fresh locked reload before any further remote or local
mutation. Directory creation/ancestor syncing and removal durability remain caller policy. On non-Unix platforms,
`os.Rename` does not guarantee atomic replacement; this package does not strengthen that guarantee.

The directory ancestry must be trusted and stable throughout each operation. Leaf checks do not protect against an
actor replacing parent directories, nor identify foreign regular files/hard links as owned. No-follow reads use a
preflight type check, platform no-follow/nonblocking flags, then descriptor validation; the flags close the symlink
race and avoid blocking on a raced-in FIFO. They do not promise never to open a raced-in device.

`CheckRegular` and `NoFollowFlags` support libraries which own their open, such as `gofrs/flock`. The preflight check
alone is insufficient. Callers MUST also validate the actual opened descriptor before reading or exposing ownership.
The lock wrapper validates the held descriptor after acquisition, preserves the library's portable lock/retry behavior,
and never removes or replaces lock files while managing ownership. Unsupported no-follow platforms fail closed.

SNMPv3 uses these operations for its bounded engine ID/boots state, preserving 0640 files and 0750 directory creation
subject to umask. S3 uses them for its 64 KiB journal, exact 0600 files and 0700 root, retaining lifetime locks, identity
validation, parent syncing and publication-uncertainty handling. Crash-orphaned legacy and randomized temps are harmless
and preserved; only a call's own unsuccessful temp is cleaned up.
