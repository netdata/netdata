# kickstart release-tag resolution tests

Regression tests for the way `packaging/installer/kickstart.sh` answers the
question *"which release does `/latest` point at?"*.

## What is being guarded

`get_redirect()` resolves a `.../releases/latest` URL by following the redirect
and taking the final path segment of the resulting URL. That segment is then
used to build download URLs.

If the server answers with an HTTP error instead of a redirect, there is no
redirect to follow — and the resolved value must not silently become the literal
string `latest`. When it does, callers build URLs such as:

```
https://github.com/netdata/netdata-nightlies/releases/download/latest/netdata-latest.tar.gz
```

which is a 404. The user then sees a download failure whose message points at
the wrong thing, several steps away from the real cause (a transient server
error during tag resolution).

This actually happened: a transient HTTP 503 from GitHub during the v2.11.0
release turned into a `404 File not found` in the installer.

## Why the tests drive caller flows, not just `get_redirect`

The original defect was not only that `get_redirect()` returned a bad value — it
was that **no caller checked its return status**. All four call sites are of the
form:

```sh
tag="$(get_redirect "...")"
export NETDATA_SOURCE_ARCHIVE_URL=".../${tag}/..."
```

The assignment's exit status is discarded, and the following `export` resets
`$?`. So a fix that only makes `get_redirect()` return non-zero changes nothing
observable. The tests therefore also exercise `set_source_archive_urls()` and
`set_static_archive_urls()` and assert both that failure propagates and that no
`/download/latest/` URL is ever constructed.

## What is *not* a failure

A mirror may legitimately serve a real `latest/` directory, with no redirect
involved. Netdata's own artifact-verification jobs in
`.github/workflows/build.yml` are built exactly that way: they create a
`latest/` directory, serve it over HTTP, and point `NETDATA_TARBALL_BASEURL` at
it.

In that layout `latest` **is** the correct answer, so resolution returning
`latest` must not be treated as an error. Only an HTTP error or an empty result
means resolution failed. There is a test for this, because an earlier version of
the fix got it wrong and would have broken those jobs.

## Cases covered

| Case | Expectation |
|---|---|
| Healthy redirect | resolves to the release tag |
| Persistent 503 | fails; never yields a bogus tag |
| 200 with no redirect (direct mirror) | succeeds, resolving to `latest` |
| 503, 503, then redirect | succeeds — `curl --retry` absorbs the transient failure |
| `--dry-run` | still resolves; the query is read-only and must not be skipped |
| Caller flow on 503 | failure propagates; no `/download/latest/` URL built |
| Caller flow when healthy | URL built exactly as before |
| wget fallback | same behaviour as curl for all three outcomes |

## Running

```sh
sh packaging/tests/kickstart-redirect/test-redirect-resolution.sh
```

Optionally pass a path to the kickstart script to test a different copy:

```sh
sh packaging/tests/kickstart-redirect/test-redirect-resolution.sh /path/to/kickstart.sh
```

Requires `python3` (used for a local fault-injection HTTP server, bound to
`127.0.0.1` on an ephemeral port) and `curl`. No network access is needed and
nothing is installed.

Exit status is `0` when every case passes, `1` otherwise.

## How the harness loads the functions

`kickstart.sh` is a single program: its function definitions sit above a
`# Main program` marker, and below that the installer runs. The harness slices
the file at that marker and sources only the definitions, so the real shipped
code is exercised without running an install.

If that marker is ever removed or renamed, the harness fails loudly with a clear
message rather than silently testing nothing.
