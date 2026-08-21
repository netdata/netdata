#!/bin/sh
# Regression tests for kickstart.sh release-tag resolution.
#
# get_redirect() resolves "which release does /latest point at?" by following a
# redirect and taking the final path segment. If the server answers with an HTTP
# error instead of a redirect, the resolved value must NOT silently become the
# literal "latest" — callers build download URLs from it, and ".../download/
# latest/netdata-latest.tar.gz" is a 404 that surfaces far from its cause.
#
# These tests drive complete caller flows (set_source_archive_urls and
# set_static_archive_urls), not get_redirect() alone, because the failure this
# guards against was a return value that no caller checked.
#
# Usage: test-redirect-resolution.sh [path-to-kickstart.sh]
#
# Requires: python3 (fault-injection HTTP server), curl.
# Exit status: 0 if every case passes, 1 otherwise.

set -eu

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "${0}")" && pwd)"
KICKSTART="${1:-${SCRIPT_DIR}/../../installer/kickstart.sh}"

[ -r "${KICKSTART}" ] || {
    echo "ERROR: cannot read kickstart script at ${KICKSTART}" >&2
    exit 2
}
command -v python3 >/dev/null 2>&1 || {
    echo "ERROR: python3 is required for the fault-injection server" >&2
    exit 2
}
command -v curl >/dev/null 2>&1 || {
    echo "ERROR: curl is required" >&2
    exit 2
}

WORK="$(mktemp -d)"
SERVER_PID=''

# Namespaced: kickstart.sh defines its own cleanup(), and sourcing it below
# would otherwise silently replace this one.
test_cleanup() {
    [ -n "${SERVER_PID}" ] && kill "${SERVER_PID}" 2>/dev/null
    cd / || :
    rm -rf "${WORK}"
    # kickstart's own scratch directory, created by set_tmpdir below.
    case "${tmpdir:-}" in
        */netdata-kickstart-*) rm -rf "${tmpdir}" ;;
    esac
}
trap test_cleanup EXIT HUP INT QUIT TERM

# ---------------------------------------------------------------- fake server

cat > "${WORK}/server.py" <<'PYEOF'
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

# Separate counters: the curl and wget flaky cases must not consume each
# other's transient failures.
flaky = {"curl": 0, "wget": 0}


class H(BaseHTTPRequestHandler):
    def route(self):
        p = self.path
        if p.startswith("/ok/latest"):
            self.send_response(302)
            self.send_header("Location", "/ok/tag/v9.9.9")
        elif p.startswith("/ok/tag/"):
            self.send_response(200)
        elif p.startswith("/fail503/latest"):
            self.send_response(503)
        elif p.startswith("/flaky/latest") or p.startswith("/flaky-wget/latest"):
            key = "wget" if p.startswith("/flaky-wget/") else "curl"
            flaky[key] += 1
            if flaky[key] <= 2:
                self.send_response(503)
            else:
                self.send_response(302)
                self.send_header("Location", "/ok/tag/v9.9.9")
        elif p.startswith("/noredirect/latest"):
            self.send_response(200)
        else:
            self.send_response(404)
        self.send_header("Content-Length", "0")
        self.end_headers()

    do_GET = route
    do_HEAD = route

    def log_message(self, *a):
        pass


if __name__ == "__main__":
    srv = HTTPServer(("127.0.0.1", 0), H)
    sys.stdout.write("%d\n" % srv.server_address[1])
    sys.stdout.flush()
    srv.serve_forever()
PYEOF

python3 "${WORK}/server.py" > "${WORK}/port" &
SERVER_PID=$!

# Wait for the server to publish its port rather than sleeping a fixed amount.
i=0
while [ ! -s "${WORK}/port" ]; do
    i=$((i + 1))
    [ "${i}" -gt 100 ] && { echo "ERROR: fault-injection server did not start" >&2; exit 2; }
    sleep 0.1
done
BASE="http://127.0.0.1:$(cat "${WORK}/port")"

# ------------------------------------------------- load kickstart's functions
#
# kickstart.sh is a single program with its function definitions above a
# "# Main program" marker. Everything above that marker is sourceable; below it
# the installer actually runs. Slice at the marker so the functions can be
# exercised directly.

MARKER_LINE="$(grep -n '^# Main program' "${KICKSTART}" | head -n 1 | cut -d: -f1)"
[ -n "${MARKER_LINE}" ] || {
    echo "ERROR: could not find the '# Main program' marker in ${KICKSTART}" >&2
    exit 2
}
head -n "$((MARKER_LINE - 3))" "${KICKSTART}" > "${WORK}/functions.sh"

# Globals the main program would normally establish. INSTALL_VERSION and
# NETDATA_OFFLINE_INSTALL_SOURCE look unused here but are read by the sourced
# kickstart functions, which branch on whether they are empty.
# shellcheck disable=SC2034
DRY_RUN=0
# shellcheck disable=SC2034
run_logfile=/dev/null
# shellcheck disable=SC2034
INTERACTIVE=0
CURL="$(command -v curl)"
# shellcheck disable=SC2034
WGET=''
# shellcheck disable=SC2034
SYSARCH='x86_64'
# shellcheck disable=SC2034
INSTALL_VERSION=''
# shellcheck disable=SC2034
NETDATA_OFFLINE_INSTALL_SOURCE=''
export DRY_RUN run_logfile INTERACTIVE CURL WGET SYSARCH

# kickstart.sh is not written to run under `set -u`, and sourcing it installs
# the installer's crash traps. Relax both for the duration of the load.
set +eu
# shellcheck disable=SC1091
. "${WORK}/functions.sh"
trap - EXIT HUP INT QUIT PIPE TERM
trap test_cleanup EXIT HUP INT QUIT TERM
setup_terminal >/dev/null 2>&1 || true

# Establish the installer tmpdir here, in the parent shell. get_redirect calls
# set_tmpdir itself, but it runs inside command substitution, so the assignment
# would not propagate back and every call would leak a new directory.
set_tmpdir >/dev/null 2>&1
cd "${WORK}" || exit 2

# ---------------------------------------------------------------------- cases

pass=0
fail=0

ok()  { pass=$((pass + 1)); printf '  PASS  %s\n' "${1}"; }
bad() { fail=$((fail + 1)); printf '  FAIL  %s\n' "${1}"; }

expect() { # <desc> <want-rc> <want-stdout> <got-rc> <got-stdout>
    if [ "${4}" = "${2}" ] && [ "${5}" = "${3}" ]; then
        ok "${1}"
    else
        bad "${1} (want rc=${2} out='${3}'; got rc=${4} out='${5}')"
    fi
}

printf '\n== get_redirect ==\n'

out="$(get_redirect "${BASE}/ok/latest" 2>/dev/null)"; rc=$?
expect "healthy redirect resolves to the release tag" 0 'v9.9.9' "${rc}" "${out}"

out="$(get_redirect "${BASE}/fail503/latest" 2>/dev/null)"; rc=$?
expect "persistent 503 fails instead of resolving to 'latest'" 6 '' "${rc}" "${out}"

# A mirror may serve a real `latest/` directory with no redirect at all. That is
# a supported layout - netdata's own artifact-verification jobs are built that
# way - so `latest` is the correct answer here, not a failure.
out="$(get_redirect "${BASE}/noredirect/latest" 2>/dev/null)"; rc=$?
expect "direct 'latest' mirror (no redirect) resolves to 'latest'" 0 'latest' "${rc}" "${out}"

out="$(get_redirect "${BASE}/flaky/latest" 2>/dev/null)"; rc=$?
expect "transient 503 then redirect succeeds (curl --retry)" 0 'v9.9.9' "${rc}" "${out}"

# Resolution is a read-only query, so it must still happen under --dry-run;
# otherwise every dry run of a stable/nightly install fails to find a tag.
DRY_RUN=1
out="$(get_redirect "${BASE}/ok/latest" 2>/dev/null)"; rc=$?
expect "resolves normally during a dry run" 0 'v9.9.9' "${rc}" "${out}"
DRY_RUN=0

printf '\n== caller flow: set_source_archive_urls ==\n'

NETDATA_TARBALL_BASEURL="${BASE}/fail503"
export NETDATA_TARBALL_BASEURL
NETDATA_SOURCE_ARCHIVE_URL=''
set_source_archive_urls 'nightly' >/dev/null 2>&1; rc=$?
if [ "${rc}" -ne 0 ]; then
    ok "failure propagates out of set_source_archive_urls"
else
    bad "set_source_archive_urls returned 0 despite a 503"
fi
case "${NETDATA_SOURCE_ARCHIVE_URL}" in
    */download/latest/*) bad "built a bogus '/download/latest/' source URL: ${NETDATA_SOURCE_ARCHIVE_URL}" ;;
    *)                   ok  "no bogus '/download/latest/' source URL was built" ;;
esac

printf '\n== caller flow: set_static_archive_urls ==\n'

NETDATA_STATIC_ARCHIVE_URL=''
set_static_archive_urls 'nightly' >/dev/null 2>&1; rc=$?
if [ "${rc}" -ne 0 ]; then
    ok "failure propagates out of set_static_archive_urls"
else
    bad "set_static_archive_urls returned 0 despite a 503"
fi
case "${NETDATA_STATIC_ARCHIVE_URL}" in
    */download/latest/*) bad "built a bogus '/download/latest/' static URL: ${NETDATA_STATIC_ARCHIVE_URL}" ;;
    *)                   ok  "no bogus '/download/latest/' static URL was built" ;;
esac

printf '\n== healthy path is unchanged ==\n'

NETDATA_TARBALL_BASEURL="${BASE}/ok"
export NETDATA_TARBALL_BASEURL
NETDATA_SOURCE_ARCHIVE_URL=''
set_source_archive_urls 'nightly' >/dev/null 2>&1; rc=$?
if [ "${rc}" -eq 0 ] && [ "${NETDATA_SOURCE_ARCHIVE_URL}" = "${BASE}/ok/download/v9.9.9/netdata-latest.tar.gz" ]; then
    ok "healthy nightly source URL is built correctly"
else
    bad "healthy nightly source URL wrong (rc=${rc} url=${NETDATA_SOURCE_ARCHIVE_URL})"
fi

printf '\n== wget fallback (used when curl is absent) ==\n'

if command -v wget >/dev/null 2>&1; then
    saved_curl="${CURL}"
    CURL=''
    WGET="$(command -v wget)"

    out="$(get_redirect "${BASE}/ok/latest" 2>/dev/null)"; rc=$?
    expect "wget: healthy redirect resolves to the release tag" 0 'v9.9.9' "${rc}" "${out}"

    out="$(get_redirect "${BASE}/noredirect/latest" 2>/dev/null)"; rc=$?
    expect "wget: direct 'latest' mirror resolves to 'latest'" 0 'latest' "${rc}" "${out}"

    out="$(get_redirect "${BASE}/fail503/latest" 2>/dev/null)"; rc=$?
    if [ "${rc}" -ne 0 ] && [ -z "${out}" ]; then
        ok "wget: persistent 503 fails instead of resolving to 'latest'"
    else
        bad "wget: persistent 503 (want rc!=0 out=''; got rc=${rc} out='${out}')"
    fi

    # The retry must come from our own loop, not from a wget option: BusyBox
    # wget has no HTTP-level retry and GNU wget needs --retry-on-http-error.
    out="$(get_redirect "${BASE}/flaky-wget/latest" 2>/dev/null)"; rc=$?
    expect "wget: transient 503 then redirect succeeds (shell retry)" 0 'v9.9.9' "${rc}" "${out}"

    CURL="${saved_curl}"
    WGET=''
else
    printf '  SKIP  wget not installed\n'
fi

printf '\n%d passed, %d failed\n' "${pass}" "${fail}"
[ "${fail}" -eq 0 ]
