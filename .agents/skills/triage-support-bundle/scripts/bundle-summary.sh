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
        --incident) INCIDENT="${2:-}"; shift 2 ;;
        -h|--help)  sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
        -*)         sb_die "unknown option: $1" ;;
        *)          INPUT="$1"; shift ;;
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
  "streaming key kept : \(if .streaming_api_key_redacted then "no" else "YES - verbatim in the collected stream config, by design" end)",
  "raw SNMP evidence  : " + (if .snmp_diagnostics == null then "not in this schema (pre-v2 bundle)" else "requested=\(.snmp_diagnostics.requested) status=\(.snmp_diagnostics.status) files=\(.snmp_diagnostics.files)" end)
' "$M"
# A pre-v2 bundle has no snmp_diagnostics object at all; treating a missing
# count as non-zero would falsely claim the bundle carries unsanitized evidence.
if [ "$(jq -r '.snmp_diagnostics.files // 0' "$M")" != "0" ]; then
    sb_warn "raw SNMP evidence present: UNSANITIZED. Handle privately; SNMP questions belong to triage-snmp-diagnostics."
fi

hdr "Log window"
# The window is not a manifest field; it survives in the journal capture origin.
WIN="$(jq -r '.files[] | select(.path|test("journal-netdata")) | .origin' "$M" 2>/dev/null \
       | grep -oE -- "-[0-9]+ hours" | head -1 || true)"
if [ -n "$WIN" ]; then
    echo "journal window  : ${WIN# } before collection"
else
    echo "journal window  : not determinable (no journal capture in this bundle)"
fi
GEN="$(jq -r '.generated_utc' "$M")"
echo "generated       : ${GEN}"
if [ -n "$INCIDENT" ]; then
    if command -v date >/dev/null 2>&1 \
       && gi="$(date -u -d "$INCIDENT" +%s 2>/dev/null)" \
       && gg="$(date -u -d "$GEN" +%s 2>/dev/null)"; then
        hrs="$(( (gg - gi) / 3600 ))"
        echo "incident        : ${INCIDENT}  (${hrs}h before collection)"
        if [ -n "$WIN" ]; then
            wh="$(echo "$WIN" | grep -oE '[0-9]+')"
            if [ "$hrs" -gt "$wh" ]; then
                sb_warn "INCIDENT IS OUTSIDE THE LOG WINDOW (${wh}h). Log silence proves nothing; ask for a bundle with a wider window."
            elif [ "$hrs" -lt 0 ]; then
                sb_warn "incident timestamp is AFTER collection; this bundle cannot contain it."
            else
                echo -e "${SB_GREEN}incident is inside the journal window${SB_NC}"
            fi
        fi
    else
        sb_warn "could not parse --incident '${INCIDENT}'"
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
    if jq -e --arg p "$1" '.files[] | select(.path==$p)' "$M" >/dev/null 2>&1; then
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

hdr "Truncated, withheld, skipped"
found=0
while IFS= read -r p; do
    [ -n "$p" ] || continue
    f="${B}/${p}"
    [ -f "$f" ] || continue
    if head -c 4096 "$f" 2>/dev/null | grep -qE 'SKIPPED: global deadline|### TRUNCATED|content withheld|was withheld|sanitization failed|capture failed'; then
        reason="$(head -c 4096 "$f" | grep -oE 'SKIPPED: global deadline[^"]*|### TRUNCATED[^#]*###|\[[^]]*withheld[^]]*\]|"error":"[^"]*"|sanitization failed[^"]*' | head -1)"
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
