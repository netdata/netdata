# nd-run

`nd-run` executes a command after applying Netdata's existing unprivileged-account and capability policy:

```text
nd-run command [args...]
nd-run --preserve-env -- command [args...]
```

The helper selects the configured Netdata account, falling back to `nobody`. An unprivileged caller that cannot
switch accounts may retain its current identity. Supplementary groups and UID/GID changes follow the same path in
both modes. Builds with libcap clear process capabilities. The command replaces the helper process with `execvp`,
using its PATH lookup and argument semantics. The helper restores ignored/blocked SIGPIPE before execution.

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
for home-relative credential files. Account selection and privilege dropping are unchanged.

Only a leading `--preserve-env` selects the new mode, and it requires `--` followed by a command. Malformed opt-in
invocations exit with status 1 without executing a command. Other invocations continue treating the first argument
as the command; there is no general option parser. Arguments after the command are passed unchanged.
Missing executables return 127; other exec failures return 126; successful exec preserves the target's exit status.
Existing callers continue using the default mode until separately migrated. A caller requiring preservation must
fail closed when the helper lacks this option; it must not retry with direct privileged execution.

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
PATH lookup, exit errors, exec replacement, SIGPIPE, and observed identities/groups. Test failures withhold environment
values. Native non-root runs verify the existing inability-to-switch path; they do not prove root privilege dropping.
Inherited groups are compared with a directly executed C probe: Python's `os.getgroups()` can report account
memberships on macOS, which may differ from the C process's supplementary groups. The patched Python group list
guards against reintroducing `os.getgroups()` as the expected process groups; the correct test does not call it.
It does not change subprocess credentials. Root transitions still check the target account's expected groups.

In a disposable Linux environment, install a compiler, Python 3 and libcap development headers. Create an unprivileged
test account, then run as root with `--user <account> --capabilities`. This additionally verifies root-to-account IDs,
supplementary groups, inability to regain root, and clearing effective/permitted/inheritable/ambient capabilities.
The capability test first proves its launcher conveys a real capability across exec. Repeat as a non-root account
to cover the inability-to-switch path. `--no-setres` exercises the portable setuid/setgid build path; `--source` accepts
an older helper source for regression comparisons. Full-agent CMake builds remain separate validation.
