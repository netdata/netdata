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
kernel interface without a libcap dependency. The command replaces the helper process with `execvp`, using its PATH
lookup and argument semantics. The helper restores ignored/blocked SIGPIPE before command or file execution.

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
