# nd-run

`nd-run` executes a command after applying Netdata's existing unprivileged-account and capability policy:

```text
nd-run command [args...]
nd-run --preserve-env -- command [args...]
```

The helper selects the configured Netdata account, falling back to `nobody`. An unprivileged caller that cannot
switch accounts may retain its current identity. All modes share one privilege-reduction path in `nd-run.c`.
Real, effective and saved UID/GID values are normalized to the resulting identity, including same-user and fallback
execution. Same-user execution retains inherited supplementary groups. Linux capabilities are cleared through the
kernel interface without a libcap dependency. Ordinary command modes replace the helper process with `execvp`, using
its PATH lookup and argument semantics. The opt-in tree supervisor forks the command and retains ownership until drain.
The helper restores ignored/blocked SIGPIPE before command or file execution.

## Environment contract

By default, the helper constructs a minimal environment:

| Variables | Value |
| --- | --- |
| `USER`, `LOGNAME` | Selected account name |
| `HOME` | Selected account's home directory |
| `SHELL` | `/bin/sh` |
| `LC_ALL` | `C` |
| `PATH`, `PWD`, `TZ`, `TZDIR` | Inherited if present |
| `TMPDIR` | Inherited if present, including an empty value; otherwise `/tmp` |

`--preserve-env` is an explicit opt-in for trusted commands that need inherited authentication, proxy or application
configuration. It preserves every inherited entry except `USER`, `LOGNAME`, `HOME`, `SHELL` and `LC_ALL`. Those five
fields use the values above, with exactly one assignment each even if the inherited environment has duplicates.
Other entries retain their bytes and order, including empty values, whitespace, newlines and equals signs.
`TMPDIR` still defaults to `/tmp` only when absent. Storage is sized from the environment, without a fixed entry cap.

Values are passed in the process environment; the helper does not expand them as shell expressions or relay them
through arguments, logs or files. A target program may interpret its own environment. Preservation does not recover
variables stripped by the operating-system loader before the helper starts, and does not preserve the caller's HOME
for home-relative credential files. Both command modes use the same account selection and privilege dropping.

A leading `--preserve-env` selects environment preservation and requires `--` followed by a command. Malformed opt-in
invocations exit with status 1 without executing a command. Outside the named helper modes, the first argument is
the command; there is no general option parser. Arguments after the command are passed unchanged.
Missing executables return 127; other exec failures return 126; successful exec preserves the target's exit status.
Existing callers continue using the default mode until separately migrated. A caller requiring preservation must
fail closed when the helper lacks this option; it must not retry with direct privileged execution.

## Private file-reader interface

The file mode performs exactly one operation and exits. It reads no commands from stdin and opens no network listener.
It runs after the same centralized privilege reduction as commands and rejects a remaining root identity. The file
module itself contains no privilege-changing code. It is compiled into `nd-run`, not installed as another executable.

```text
nd-run --file-reader read regular|stream <byte-limit> <path>
nd-run --file-reader stat <path>
```

Arguments have fixed positions; pass the path as one argument without a shell. The byte limit is unsigned decimal;
zero means unlimited. Regular reads open nonblocking, validate the opened descriptor with `fstat`, and reject
non-regular objects. Symlinks to regular files are accepted. The limit is checked both against file size and while
reading, so growing or underreported files cannot bypass it. A limit of 1048576 preserves the 1 MiB credential policy.
Stream reads retain ordinary blocking file/stream behavior and may also use a byte limit.

Read stdout contains raw file bytes with no framing. Stat follows symlinks and writes one line containing signed Unix
seconds, a space, nanoseconds and a newline. File results use exactly one stderr line:

```text
NDFILE <result> <errno>
```

| Result | Meaning |
| --- | --- |
| 0 | Success |
| 1 | Open failed |
| 2 | Stat failed |
| 3 | Read failed |
| 4 | Close failed |
| 5 | Opened object is not a regular file |
| 6 | File exceeds the byte limit |

Native failures carry their numeric errno; success and file-policy failures carry zero. Exit status is zero only
for success. Argument, startup, privilege-reduction, signal and output failures may exit without a valid result.
Diagnostics never include file contents or parser fragments. `nd-file-reader.h` owns this private contract.

Consumers MUST require a successful process exit and exactly `NDFILE 0 0` before accepting buffered data. On error,
discard any partial stdout. Streaming consumers may have already received bytes and MUST report terminal failure
instead of clean EOF. A missing/malformed result or exit/result disagreement is a helper failure, not a missing file.
Callers own cancellation and reaping: a blocked FIFO open/read requires terminating the specific child, not merely
closing its pipes. These rules apply per invocation; there is no persistent session or restart protocol.

## Private process-tree supervision

```text
nd-run --supervise-tree <control-fd> <status-fd> [--completion <directory-fd> <id>] -- command [args...]
```

This Linux-only mode applies the existing identity and minimal-environment policy, then remains a per-run subreaper.
Before launching the payload it sets and verifies inherited `no_new_privs`, preventing setuid/setgid executables and
file capabilities from granting privileges that would invalidate the supervisor's signal authority. Setup failure
refuses launch; legacy exec and file-reader modes retain their existing policies.
Control and status must be distinct private pipes above descriptor 2, with no inherited aliases. The payload closes
both channels before exec. Control input/EOF, signal requests, status-reader loss or command exit starts descendant
termination and reaping. Detached process groups are included through orphan adoption; no process-wide scan is used.

A final `NDTREE1 <raw-wait-status>` line follows kernel `ECHILD`, including after a failed command. The supervisor exits
with the normalized command status only when that command-status frame is successfully delivered. An unavailable
command outcome or frame-delivery failure exits 126 without valid command-status evidence.
`NDTREE1 unavailable` plus exit 126 means verified empty setup refusal before launch.
Missing/invalid completion evidence does not establish cleanup. Callers must join the helper and validate its private
frame separately from command output; they must not kill the supervisor to implement cancellation or treat a deadline
as successful drain. See the [Go helper contract](/src/go/plugin/go.d/docs/helper-packages.md#external-commands) for
caller ownership, platform prerequisites and the supervisor-loss limitation.

Run real ownership fixtures in a disposable Linux environment:

```sh
python3 tests/nd-run/test_process_tree.py
```

### Persistent tree completion

Callers retaining resources across process restarts may supply `--completion`. It is opt-in; ordinary tree
supervision is unchanged. The trusted resource owner first creates a fresh private directory (mode `0700`) containing
mode `0600` regular files `publication.lock` (empty) and `pending` (`NDTREE1 pending <id>\n`). The ID is 64 lowercase
hexadecimal characters, unique to this invocation. Persist the files, directory entry and resource-to-ID binding
before launch. Pass an already-open directory descriptor, never a payload-selected path.

The helper validates the expected pending record and exclusively creates and persists a `started` marker, then
acquires the exclusive `publication.lock` before forking. The marker claims this invocation even while a recovery
reader delays launch by holding the lock. A started/used invocation cannot run again, even after supervisor loss. Protocol
aliases are rejected, and all record handles are closed in the payload. Only after `waitpid` establishes ECHILD does
the helper write/fsync `complete.pending`, atomically publish `complete` (`NDTREE1 drained <id>\n`), sync the directory,
and release the publication lock. This happens before status-pipe delivery and also after a verified prelaunch setup
refusal. Command failure does not prevent verified cleanup. Completion certifies quiescence, not command success.

Recovery requires the exact matching pending/completion records and an acquired publication lock; acquiring a lock
alone never proves drainage. Missing, malformed or unreadable evidence remains uncertain. Completion publication
failure does not change the live status-frame result: a living owner may still use that validated drainage proof.
Killing the helper, or losing the caller between pin creation and launch, may leave an uncertain record indefinitely.
Do not infer completion from a PID, timeout, reboot of a container, or missing helper. Any host-reboot recovery belongs
to the resource owner's separately established host-local storage contract.

After releasing its record lock, the helper never accesses the directory again. It must reside outside any resource
being retired and must not access that resource after publishing completion. The owner may remove completed records;
it must serialize its own acquisition/retirement policy. This protocol uses the existing trusted-command model,
not isolation against malicious same-account code. It neither implements retention policy nor grants new privileges.

## Regression tests

Run the standalone suite from the repository root with Python 3 and a C compiler:

```sh
python3 tests/nd-run/test_nd_run.py -v
# Linux: AddressSanitizer and UndefinedBehaviorSanitizer
python3 tests/nd-run/test_nd_run.py --cc clang --cflags='-fsanitize=address,undefined' -v
# macOS: UndefinedBehaviorSanitizer
python3 tests/nd-run/test_nd_run.py --cc clang --cflags='-fsanitize=undefined' -v
```

The suite builds the actual helper and a C probe in a temporary directory, using synthetic environments only.
It covers default and preserved environments, duplicate controlled fields, account fallback, argument boundaries,
PATH lookup, exit errors, exec replacement, SIGPIPE, observed identities/groups and one-shot file operations. Test failures
withhold environment values. Native non-root runs verify the existing inability-to-switch path; they do not prove root privilege dropping.
Inherited groups are compared with a directly executed C probe: Python's `os.getgroups()` can report account
memberships on macOS, which may differ from the C process's supplementary groups. The patched Python group list
guards against reintroducing `os.getgroups()` as the expected process groups; the correct test does not call it.
It does not change subprocess credentials. Root transitions still check the target account's expected groups.

In a disposable Linux environment, install a compiler and Python 3. Create an unprivileged
test account, then run as root with `--user <account>`. This additionally verifies root-to-account IDs,
supplementary groups, inability to regain root, and clearing effective/permitted/inheritable/ambient capabilities.
The probes use kernel capability APIs without libcap. Authority tests prove elevated access to synthetic fixtures
before verifying denial through the helper, including ambient capabilities and setuid/file-capability parents.
Repeat as a non-root account to cover the inability-to-switch path. `--no-setres` exercises the portable
setreuid/setregid build path; `--source` accepts an older helper source for regression comparisons. Full-agent CMake
builds remain separate validation.

## Restricted privileged tegrastats supervision

Linux `ndsudo tegrastats` is a private supervised command. It accepts no additional arguments and never enters the
legacy PATH/template execution path (`--test tegrastats` is not supported). It runs only
`/usr/bin/tegrastats --interval 1000`. Existing ndsudo commands retain their behavior.

The regular ELF and every directory component in `/usr/bin/tegrastats` must be root-owned and reject group/world writes.
Symlinks, scripts, setuid/setgid modes and file capabilities are rejected. Directory traversal uses pinned descriptors and
`O_NOFOLLOW`; descriptor execution pins the checked file through exec. This deliberately supports the fixed vendor
installation rather than arbitrary relocated wrappers. Real/effective/saved IDs become root, supplementary groups
are cleared, cwd is `/`, and the payload receives an empty environment and only its standard descriptors.

The caller supplies two distinct private pipes: fd 3 is the control read end; fd 4 is the status write end. Inherited
aliases are rejected using self-descriptor enumeration. The payload cannot inherit either pipe. Status is nonblocking
and separate from stdout/stderr. Closing control, writing control input, losing the status reader, a termination signal,
or payload exit initiates shutdown. The supervisor sends SIGKILL to the owned payload group while its leader is still
pinned, then reaps direct/adopted children until `ECHILD`. It never signals that group after reaping the leader.

The Linux subreaper does not require `/proc/*/children`. It does require procfs self-descriptor enumeration. It does
not enumerate or kill escaped groups: an escaped live descendant keeps cleanup unfinished. No timeout is drain proof.
The child sets parent-death SIGKILL after credential setup, with a parent race check; this protects the direct payload
on supervisor loss, not arbitrary descendants. Callers must not kill the supervisor to enforce their waiting budget.

The versioned private status stream is one of:

- `NDSUP1 refused <reason>\n`, exit 126: no payload was forked. Reasons are `privilege`, `missing`, `unsafe`,
  `arguments`, or `setup`. Only `privilege` and `missing` permit the caller to consider unprivileged fallback.
- `NDSUP1 started\n`, followed by `NDSUP1 cleaned <raw-wait-status>\n`: the close-on-exec error pipe acknowledged
  execution, then kernel wait exhaustion proved cleanup. The helper exit code must match the raw payload status.
- `NDSUP1 failed\n`, followed by the same cleaned frame: child setup/exec failed. Cleanup can be verified, but this
  is not permission to fall back.

Missing, malformed or contradictory evidence means unverified cleanup, even if the helper exits successfully.
No command-output text is part of this protocol. Exec acknowledgement does not prove usable telemetry or full fields.

The Go `ndexec.Acquisition` adapter returns the existing `*Process` type. Its immutable namespace/job owner excludes
simultaneous starts across both supervised and unprivileged routes within the plugin. It retains ownership while
privileged cleanup is unfinished or unverified; valid late completion releases it. Unprivileged runs retain their
existing weaker group-signal/leader-reap contract. Separate names and renamed jobs are independent. These in-memory
leases do not survive plugin restart and are not a host singleton or persistent containment mechanism.

Run the real setuid-boundary fixtures only inside a disposable root Linux Docker container with a C compiler. They
refuse to replace an existing `/usr/bin/tegrastats`, install a synthetic ELF there, and run clients as UID 65534:

```sh
cd src/go
NETDATA_TEST_NDSUDO_SUPERVISION=1 go test -race -count=1 ./plugin/go.d/pkg/ndexec -run 'Test(Acquisition|Supervised)'
```

These fixtures establish helper/API behavior. They do not certify NVIDIA vendor behavior, board coverage, packaging,
or on-device overhead; those require an installed collector run on hardware. Collector adoption is a separate change.
