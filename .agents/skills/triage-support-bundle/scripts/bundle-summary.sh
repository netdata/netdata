#!/usr/bin/env bash
# Inventory a Netdata support bundle before reading any single file.
#
# Answers the questions that decide how every other file is read: which
# implementation produced it, whether the agent and its API were alive, what the
# log window was, what is truncated or withheld, and whether raw SNMP evidence
# is present. Absence is the point - reading files without this is how a missing
# artifact becomes a false finding.
#
# Usage:
#   bundle-summary.sh <bundle.tar.zst|bundle.zip|extracted-dir> [--incident <ISO8601>]
#
# Reads only. Writes nothing into the bundle.

_self="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source-path=SCRIPTDIR
# shellcheck source=.agents/skills/triage-support-bundle/scripts/_lib.sh
. "${_self}/_lib.sh"

INPUT=""
INCIDENT=""

while [ $# -gt 0 ]; do
    case "$1" in
        --incident) [ $# -ge 2 ] || sb_die "--incident needs an ISO8601 timestamp"
                    INCIDENT="$2"; shift 2 ;;
        -h|--help)  sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
        -*)         sb_die "unknown option: $1" ;;
        *)          [ -z "$INPUT" ] || sb_die "only one bundle may be given (got '$INPUT' and '$1')"
                    INPUT="$1"; shift ;;
    esac
done

[ -n "$INPUT" ] || sb_die "usage: bundle-summary.sh <bundle> [--incident <ISO8601>]"
sb_need jq

sb_resolve_bundle "$INPUT"
trap sb_cleanup_bundle EXIT
B="$SB_BUNDLE_DIR"
M="${B}/MANIFEST.json"
[ -f "$M" ] || sb_die "no MANIFEST.json in ${B}"

hdr() { echo -e "\n${SB_CYAN}== $* ==${SB_NC}"; }

hdr "Identity"
jq -r '
  "schema          : \(.schema)",
  "tool version    : \(.tool_version)   \(if (.tool_version|test("windows")) then "(WINDOWS bundle - see windows.md before reading any absence)" else "(POSIX bundle)" end)",
  "generated (UTC) : \(.generated_utc)",
  "collection time : \(.runtime_seconds)s"
' "$M"

hdr "Agent state at collection"
jq -r '
  "process running : \(.agent_running)",
  "API reachable   : \(.agent_api_reachable)\(if .agent_api_reachable then "" else "   <-- runtime area is a marker, NOT proof the agent was down" end)",
  "container       : \(.is_container)"
' "$M"

hdr "Sanitization"
jq -r '
  "PII pseudonymized  : \(.pii_obfuscated)",
  "secrets redacted   : \(.secrets_redacted)",
  "streaming key kept : " + (if .streaming_api_key_redacted == null then "unknown (field absent; pre-1.1.0 bundle)" elif .streaming_api_key_redacted then "no" else "YES - verbatim in the collected stream config, by design" end),
  "raw SNMP evidence  : " + (if .snmp_diagnostics == null then "not in this schema (pre-v2 bundle)" else "requested=\(.snmp_diagnostics.requested) status=\(.snmp_diagnostics.status) files=\(.snmp_diagnostics.files)" end)
' "$M"
# A pre-v2 bundle has no snmp_diagnostics object at all; treating a missing
# count as non-zero would falsely claim the bundle carries unsanitized evidence.
if [ "$(jq -r '.snmp_diagnostics.files // 0' "$M")" != "0" ]; then
    sb_warn "raw SNMP evidence present: UNSANITIZED. Handle privately; SNMP questions belong to triage-snmp-diagnostics."
fi

hdr "Log window"
# The window is not a manifest field; it survives in the journal capture origin.
# POSIX records the window in the journal capture origin; Windows has no journal
# and records it on the merged event-log capture instead.
WIN="$(jq -r '.files[] | select(.path|test("journal-netdata|eventlog-netdata")) | .origin' "$M" 2>/dev/null \
       | grep -oiE -- "-?[0-9]+ ?h(ours)?" | grep -oE '[0-9]+' | head -1 || true)"
[ -n "$WIN" ] && WIN="-${WIN} hours"
if [ -n "$WIN" ]; then
    echo "journal window  : ${WIN# } before collection"
else
    echo "journal window  : not determinable (no journal capture in this bundle)"
fi
GEN="$(jq -r '.generated_utc' "$M")"
echo "generated       : ${GEN}"
if [ -n "$INCIDENT" ]; then
    # BSD/macOS date has no -d; fall back to GNU-style gdate, then to python.
    to_epoch() {
        date -u -d "$1" +%s 2>/dev/null \
          || gdate -u -d "$1" +%s 2>/dev/null \
          || python3 -c 'import sys,datetime as d;t=sys.argv[1].replace("Z","+00:00");print(int(d.datetime.fromisoformat(t).timestamp()))' "$1" 2>/dev/null
    }
    # errexit would abort on the failing substitution and swallow the warning
    # this branch exists to print.
    gi=""; gg=""
    gi="$(to_epoch "$INCIDENT" || true)"
    gg="$(to_epoch "$GEN" || true)"
    if [ -n "$gi" ] && [ -n "$gg" ]; then
        delta=$(( gg - gi ))
        # Compare seconds, not truncated hours: an incident up to 59 minutes
        # AFTER collection used to divide down to 0 and read as "inside".
        if [ "$delta" -lt 0 ]; then
            sb_warn "incident timestamp is AFTER collection by $(( -delta / 60 ))m; this bundle cannot contain it."
        else
            echo "incident        : ${INCIDENT}  ($(( delta / 3600 ))h $(( (delta % 3600) / 60 ))m before collection)"
            if [ -n "$WIN" ]; then
                wh="$(printf '%s' "$WIN" | grep -oE '[0-9]+')"
                if [ "$delta" -gt $(( wh * 3600 )) ]; then
                    sb_warn "INCIDENT IS OUTSIDE THE JOURNAL WINDOW (${wh}h). Journal silence proves nothing; on-disk log tails are not window-bounded, so check those before concluding."
                else
                    echo -e "${SB_GREEN}incident is inside the journal window${SB_NC}"
                fi
            else
                sb_warn "no journal capture, so the window cannot be checked; judge log evidence on its own timestamps."
            fi
        fi
    else
        sb_warn "could not parse --incident '${INCIDENT}' (expected ISO8601, e.g. 2026-09-13T19:30:00Z)"
    fi
else
    sb_warn "no --incident given: you cannot judge whether silence is meaningful without it."
fi

hdr "Contents by area"
for d in 01-system 02-install 03-process 04-config 05-logs 06-state 07-runtime 08-network 09-permissions; do
    n="$(jq -r --arg d "$d" '[.files[] | select(.path|startswith($d))] | length' "$M")"
    b="$(jq -r --arg d "$d" '[.files[] | select(.path|startswith($d)) | .bytes] | add // 0' "$M")"
    printf '  %-16s %3s files  %10s bytes\n' "$d" "$n" "$b"
done

hdr "Key artifacts"
check() { # path, note
    # Manifest membership is not presence: a row whose file is missing on disk
    # is a broken bundle, not an available artifact.
    if jq -e --arg p "$1" '.files[] | select(.path==$p)' "$M" >/dev/null 2>&1 \
       && [ -f "${B}/$1" ] && [ -r "${B}/$1" ]; then
        printf '  %-42s %spresent%s\n' "$1" "$SB_GREEN" "$SB_NC"
    else
        printf '  %-42s %sabsent%s   %s\n' "$1" "$SB_YELLOW" "$SB_NC" "$2"
    fi
}
check "06-state/status-file.json"        "no crash/exit record - check whether the host ever wrote one"
check "07-runtime/dyncfg-tree.json"      "NO collector job state - API down, or a bundle predating this capture"
check "04-config/effective-netdata.conf" "no merged config - fall back to on-disk files and say so"
check "06-state/health-silencers.json"   "POSIX-only; on Windows 'not silenced' is unprovable"
check "07-runtime/stream-info.json"      "no streaming diagnostics"
check "08-network/netdata-sockets.txt"   "no socket inventory"

# The manifest is attacker-controlled data, not a trusted index: a crafted row
# can name an absolute path or climb out of the bundle, and every loop below
# builds a filesystem path from it. Accept only paths that stay inside.
sb_manifest_path_ok() { # relative path from the manifest
    case "$1" in
        ""|/*|[A-Za-z]:*|*$'\n'*) return 1 ;;
        ..|../*|*/../*|*/..) return 1 ;;
    esac
    return 0
}

hdr "Truncated, withheld, skipped"
found=0
# A capped file copy carries no in-body marker at all: the only signal is the
# manifest origin. Report those first, or the section claims a clean bundle
# while on-disk logs are silently incomplete. Withheld symlink and reparse-point
# sources DO write a body marker, so they are left to the scan below rather than
# reported twice.
while IFS=$'\t' read -r p origin; do
    [ -n "$p" ] || continue
    printf '  %s%s%s\n      manifest origin: %s\n' "$SB_YELLOW" "$p" "$SB_NC" "$origin"
    found=1
done < <(jq -r '.files[]
           | select(.kind == "file")
           | select(.origin | test("line-aligned"))
           | "\(.path)\t\(.origin)"' "$M")
while IFS= read -r p; do
    [ -n "$p" ] || continue
    if ! sb_manifest_path_ok "$p"; then
        printf '  %s%s%s\n      manifest path escapes the bundle - not read\n' "$SB_RED" "$p" "$SB_NC"
        found=1; continue
    fi
    f="${B}/${p}"
    if [ ! -f "$f" ] || [ ! -r "$f" ]; then continue; fi
    # Withheld and skipped markers replace the body (head); a TRUNCATED marker is
    # APPENDED after up to megabytes of output (tail). Scanning one end misses
    # half the cases.
    ends="$( { head -c 4096 "$f"; printf '\n'; tail -c 4096 "$f"; } 2>/dev/null )"
    if printf '%s' "$ends" | grep -qE 'SKIPPED: global deadline|### TRUNCATED|content withheld|was withheld|sanitization failed|capture failed'; then
        reason="$(printf '%s' "$ends" | grep -oE 'SKIPPED: global deadline[^"]*|### TRUNCATED[^#]*###|\[[^]]*withheld[^]]*\]|"error":"[^"]*"|sanitization failed[^"]*' | head -1)"
        printf '  %s%s%s\n      %s\n' "$SB_YELLOW" "$p" "$SB_NC" "${reason:-marker present}"
        found=1
    fi
done < <(jq -r '.files[].path' "$M")
[ "$found" -eq 0 ] && echo "  none"

hdr "Manifest / on-disk parity"
# LC_ALL=C throughout: comm requires both inputs in the same collation order,
# and a locale-aware sort disagrees with the byte order the paths are compared in.
rows="$(jq -r '.files[].path' "$M" | LC_ALL=C sort)"
disk="$(cd "$B" && find . -type f ! -name MANIFEST.json | sed 's|^\./||' | LC_ALL=C sort)"
only_m="$(LC_ALL=C comm -23 <(printf '%s\n' "$rows") <(printf '%s\n' "$disk"))"
only_d="$(LC_ALL=C comm -13 <(printf '%s\n' "$rows") <(printf '%s\n' "$disk"))"
echo "  manifest rows : $(printf '%s\n' "$rows" | grep -c .)"
echo "  on-disk files : $(printf '%s\n' "$disk" | grep -c .)"
if [ -n "$only_m" ] || [ -n "$only_d" ]; then
    [ -n "$only_m" ] && { echo -e "  ${SB_RED}in manifest, not on disk:${SB_NC}"; printf '    %s\n' "$only_m"; }
    [ -n "$only_d" ] && { echo -e "  ${SB_RED}on disk, not in manifest:${SB_NC}"; printf '    %s\n' "$only_d"; }
    sb_warn "parity broken - this bundle may be incomplete or modified after collection."
else
    echo -e "  ${SB_GREEN}parity OK${SB_NC}"
fi

echo -e "\n${SB_GRAY}Next: SKILL.md 'Choose The Investigation'. Resolve every absence through bundle-map.md${SB_NC}"
echo -e "${SB_GRAY}before it becomes a finding, and check false-signals.md before reporting.${SB_NC}"
