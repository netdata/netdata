#!/usr/bin/env bash
# Classify a Netdata support bundle and draft an internal triage note.
#
# Pipeline: a deterministic extractor reduces the bundle to a bounded evidence
# pack; a model classifies and drafts from that pack plus the reporter's own
# words; a deterministic gate decides what is emitted. The model never sees the
# raw bundle and never decides whether its own answer is publishable.
#
# Usage:
#   analyze-bundle.sh <bundle> [--ticket <file>] [--incident <ISO8601>]
#                     [--out <dir>] [--pack-only] [--min-confidence <0..1>]
#
# Emits <out>/pack.json, <out>/verdict.json and <out>/note.md.
# verdict.json is the contract a helpdesk connector consumes; it carries role
# KEYS (agent, cloud-backend, cloud-frontend), never people.

_self="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./_lib.sh
. "${_self}/_lib.sh"

SKILL_DIR="$(cd "${_self}/.." && pwd)"
INPUT=""; TICKET=""; INCIDENT=""; OUT=""; PACK_ONLY=0; MIN_CONF="0.5"

while [ $# -gt 0 ]; do
    case "$1" in
        --ticket)         TICKET="${2:-}"; shift 2 ;;
        --incident)       INCIDENT="${2:-}"; shift 2 ;;
        --out)            OUT="${2:-}"; shift 2 ;;
        --min-confidence) MIN_CONF="${2:-}"; shift 2 ;;
        --pack-only)      PACK_ONLY=1; shift ;;
        --selftest)       echo "credential:"; sb_selftest_no_token_leak; echo "gate:"; sb_selftest_gate; exit $? ;;
        -h|--help)        sed -n '2,17p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
        -*)               sb_die "unknown option: $1" ;;
        *)                INPUT="$1"; shift ;;
    esac
done

[ -n "$INPUT" ] || sb_die "usage: analyze-bundle.sh <bundle> [--ticket <file>] [--incident <ISO8601>]"
sb_need jq; sb_need curl

sb_resolve_bundle "$INPUT"
trap sb_cleanup_bundle EXIT
B="$SB_BUNDLE_DIR"; M="${B}/MANIFEST.json"
[ -f "$M" ] || sb_die "no MANIFEST.json in ${B}"

if [ -z "$OUT" ]; then OUT="$(sb_audit_dir)/$(date -u +%Y%m%d-%H%M%S)"; fi
mkdir -p "$OUT"; chmod 700 "$OUT"

# --------------------------------------------------------------------------
# 1. Evidence pack - bounded, derived, already sanitized by the collector
# --------------------------------------------------------------------------

# Bounded read of a bundle file: never let one artifact dominate the pack.
grab() { # path, max-bytes
    local f="${B}/$1" n="${2:-4000}"
    [ -f "$f" ] || { printf ''; return 0; }
    head -c "$n" "$f"
}

# Log lines worth showing: errors and warnings, most recent last, bounded.
loglines() { # path, max-lines
    local f="${B}/$1" n="${2:-40}"
    [ -f "$f" ] || { printf ''; return 0; }
    grep -aiE 'error|warn|fatal|cannot|failed|denied|refus|reject' "$f" 2>/dev/null | tail -n "$n" || true
}

WIN_H="$(jq -r '.files[] | select(.path|test("journal-netdata")) | .origin' "$M" 2>/dev/null \
         | grep -oE -- '-[0-9]+ hours' | grep -oE '[0-9]+' | head -1 || true)"
GEN="$(jq -r '.generated_utc' "$M")"

INSIDE="null"
if [ -n "$INCIDENT" ] && [ -n "$WIN_H" ]; then
    if gi="$(date -u -d "$INCIDENT" +%s 2>/dev/null)" && gg="$(date -u -d "$GEN" +%s 2>/dev/null)"; then
        hrs=$(( (gg - gi) / 3600 ))
        if [ "$hrs" -le "$WIN_H" ] && [ "$hrs" -ge 0 ]; then INSIDE="true"; else INSIDE="false"; fi
    fi
fi

# Absent key artifacts, each with the reason absence is ambiguous here.
absent_json() {
    local win="" p reason
    # A row ending in "/" names a directory: match any path under it. Testing a
    # directory as an exact path never matches, so it would always report absent.
    while IFS='|' read -r p reason; do
        [ -n "$p" ] || continue
        if [ "${p%/}" != "$p" ]; then
            jq -e --arg p "$p" 'any(.files[]; .path | startswith($p))' "$M" >/dev/null 2>&1 && continue
        else
            jq -e --arg p "$p" 'any(.files[]; .path == $p)' "$M" >/dev/null 2>&1 && continue
        fi
        win="${win}$(jq -nc --arg p "$p" --arg r "$reason" '{artifact:$p, why_absence_is_ambiguous:$r}')"$'\n'
    done <<'ROWS'
06-state/status-file.json|No crash record was found on disk. The host may never have written one; this is not evidence the agent never exited badly.
07-runtime/dyncfg-tree.json|Collector job state is unavailable. Either the agent API was unreachable, or the bundle predates this capture. It is NOT evidence that no jobs exist.
04-config/effective-netdata.conf|The merged running config is unavailable (API unreachable). On-disk config may differ from what the agent used.
06-state/health-silencers.json|POSIX-only artifact. On a Windows bundle its absence proves nothing about whether alerts were silenced.
06-state/dyncfg/|POSIX-only. UI-created jobs and UI-edited alerts are invisible on Windows bundles.
05-logs/journal-namespace-netdata.txt|The agent's own journal namespace was not captured; the unit journal alone shows very little on systemd.
ROWS
    printf '%s' "$win" | jq -sc '.'
}

# Files whose body was truncated, withheld or skipped.
degraded_json() {
    local p f reason acc=""
    while IFS= read -r p; do
        [ -n "$p" ] || continue
        f="${B}/${p}"; [ -f "$f" ] || continue
        reason="$(head -c 4096 "$f" 2>/dev/null | grep -aoE 'SKIPPED: global deadline[^"]*|### TRUNCATED[^#]*###|\[[^]]*withheld[^]]*\]|"error":"[^"]*"' | head -1 || true)"
        [ -n "$reason" ] || continue
        acc="${acc}$(jq -nc --arg p "$p" --arg r "$reason" '{artifact:$p, state:$r}')"$'\n'
    done < <(jq -r '.files[].path' "$M")
    printf '%s' "$acc" | jq -sc '.'
}

JOBS='null'
if [ -f "${B}/07-runtime/dyncfg-tree.json" ]; then
    JOBS="$(jq -c '
      (.tree // {}) | to_entries | map(
        {path: .key,
         states: (.value | to_entries | map(.value.status) | group_by(.) | map({(.[0]): length}) | add),
         failing: (.value | to_entries | map(select(.value.status != "accepted" and .value.status != "running") | .key) | .[0:20])}
      )' "${B}/07-runtime/dyncfg-tree.json" 2>/dev/null || echo 'null')"
fi

# Which configuration the bundle carries, by name. The contents would blow the
# budget, but knowing WHICH files exist tells the model what can still be asked
# for - the first real run flagged this gap.
CONFIG_INV="$(jq -c '[.files[].path | select(startswith("04-config/") or startswith("06-state/dyncfg/"))]' "$M")"

TICKET_TEXT=""
[ -n "$TICKET" ] && { [ -f "$TICKET" ] || sb_die "no such ticket file: $TICKET"; TICKET_TEXT="$(head -c 8000 "$TICKET")"; }

jq -n \
  --slurpfile manifest "$M" \
  --arg incident "${INCIDENT:-}" \
  --argjson inside "$INSIDE" \
  --arg window_hours "${WIN_H:-}" \
  --arg summary "$(grab summary.txt 6000)" \
  --arg status_file "$(grab 06-state/status-file.json 6000)" \
  --arg kernel "$(grab 01-system/kernel-messages.txt 2500)" \
  --arg collector_log "$(loglines 05-logs/collector.log 40)" \
  --arg daemon_log "$(loglines 05-logs/journal-namespace-netdata.txt 40)" \
  --arg error_log "$(loglines 05-logs/error.log 30)" \
  --arg eventlog "$(loglines 05-logs/eventlog-netdata.txt 40)" \
  --arg sockets "$(grab 08-network/netdata-sockets.txt 2500)" \
  --arg cloud_probe "$(grab 08-network/cloud-connectivity.txt 1500)" \
  --arg install "$(grab 02-install/install-type.txt 800)" \
  --arg perms "$(loglines 09-permissions/plugins-d.txt 30)" \
  --arg effective_db "$(grep -aA12 '^\[db\]' "${B}/04-config/effective-netdata.conf" 2>/dev/null | head -c 1200 || true)" \
  --argjson config_inventory "$CONFIG_INV" \
  --argjson jobs "$JOBS" \
  --argjson absent "$(absent_json)" \
  --argjson degraded "$(degraded_json)" \
  --arg ticket "$TICKET_TEXT" \
  '{
     schema: "netdata-bundle-evidence/v1",
     bundle: {
       tool_version:  $manifest[0].tool_version,
       platform:      (if ($manifest[0].tool_version|test("windows")) then "windows" else "posix" end),
       generated_utc: $manifest[0].generated_utc,
       agent_running: $manifest[0].agent_running,
       api_reachable: $manifest[0].agent_api_reachable,
       is_container:  $manifest[0].is_container,
       snmp:          $manifest[0].snmp_diagnostics
     },
     window: {hours: $window_hours, incident: $incident, incident_inside_window: $inside},
     ticket_text: $ticket,
     summary_txt: $summary,
     crash_record: $status_file,
     collector_jobs: $jobs,
     logs: {collector: $collector_log, daemon_namespace: $daemon_log, error: $error_log, windows_eventlog: $eventlog},
     system: {kernel_messages: $kernel, install_type: $install},
     config: {files_present: $config_inventory, effective_db_section: $effective_db},
     network: {sockets: $sockets, cloud_probe: $cloud_probe},
     permissions: $perms,
     absent_key_artifacts: $absent,
     degraded_artifacts: $degraded
   }' > "${OUT}/pack.json"

PACK_BYTES="$(wc -c < "${OUT}/pack.json")"
echo -e "${SB_CYAN}evidence pack${SB_NC}: ${OUT}/pack.json (${PACK_BYTES} bytes)"
[ "$PACK_ONLY" -eq 1 ] && exit 0

# --------------------------------------------------------------------------
# 2. Classify - the skill's own guardrail documents are the system prompt
# --------------------------------------------------------------------------

SYS="$(cat "${SKILL_DIR}/prompts/triage-system-prompt.md" \
            "${SKILL_DIR}/false-signals.md" \
            "${SKILL_DIR}/evidence-limits.md")"

SCHEMA='{
  "class": "one of alerts|no-data|crash|performance|retention|streaming|cloud|dashboard|permissions|install|container|windows|config|snmp|unknown",
  "confidence": 0.0,
  "summary": "two or three sentences: the finding, then what it rests on",
  "route": {"role": "agent|cloud-backend|cloud-frontend|unknown", "confidence": 0.0, "rationale": "which input drove this"},
  "tags": ["short-kebab-case", "..."],
  "evidence": [{"artifact": "bundle/path", "observation": "what it shows"}],
  "missing_evidence": ["what could not be checked and why"],
  "next_checks": ["the smallest next step, or what to ask the customer for"],
  "note_markdown": "the internal note body"
}'

REQ="$(mktemp "${TMPDIR:-/tmp}/sb-req.XXXXXX")"
jq -n --arg model "${NETDATA_LLM_MODEL:-}" --arg sys "$SYS" --arg schema "$SCHEMA" \
      --rawfile pack "${OUT}/pack.json" '
  {model: (if $model == "" then "MODEL_FROM_ENV" else $model end),
   temperature: 0,
   response_format: {type: "json_object"},
   messages: [
     {role: "system", content: $sys},
     {role: "user", content: ("Return exactly one JSON object with this shape:\n" + $schema +
        "\n\nEvidence pack:\n" + $pack)}
   ]}' > "$REQ"

# The model name lives in .env; patch it in without the credential leaving the lib.
if [ -z "${NETDATA_LLM_MODEL:-}" ]; then
    _m="$(grep -E '^NETDATA_LLM_MODEL=' "$(sb_repo_root)/.env" 2>/dev/null | cut -d= -f2- | tr -d '"' || true)"
    [ -n "$_m" ] && jq --arg m "$_m" '.model = $m' "$REQ" > "${REQ}.2" && mv "${REQ}.2" "$REQ"
fi

echo -e "${SB_GRAY}querying the model...${SB_NC}" >&2
RAW="$(sb_llm_chat "$REQ")" || { rm -f "$REQ"; exit 1; }
rm -f "$REQ"

# Models sometimes fence JSON despite being asked not to.
CLEAN="$(printf '%s' "$RAW" | sed -e 's/^```json//' -e 's/^```//' -e 's/```$//')"
printf '%s' "$CLEAN" | jq -e '.' >/dev/null 2>&1 \
    || { printf '%s' "$RAW" > "${OUT}/model-raw.txt"; sb_die "model did not return valid JSON; raw reply saved to ${OUT}/model-raw.txt"; }

# --------------------------------------------------------------------------
# 3. Deterministic gate - the model never decides whether it is publishable
# --------------------------------------------------------------------------

HAS_TICKET=0; [ -n "$TICKET_TEXT" ] && HAS_TICKET=1

VERDICT="$(printf '%s' "$CLEAN" | sb_apply_gate "$HAS_TICKET" "$MIN_CONF" "$(jq -c '{tool_version, generated_utc, agent_running, agent_api_reachable}' "$M")")"


printf '%s' "$VERDICT" | jq '.' > "${OUT}/verdict.json"

# --------------------------------------------------------------------------
# 4. Render the note the connector posts
# --------------------------------------------------------------------------

{
  echo "**Automated bundle triage** — generated from the attached support bundle. Verify before acting."
  echo
  if [ "$(jq -r '.gate.publish' "${OUT}/verdict.json")" = "true" ]; then
      jq -r '
        "**Finding:** " + .classification.summary, "",
        "**Class:** `" + .classification.class + "`  •  **Confidence:** " + (.classification.confidence|tostring),
        "**Suggested owner:** `" + .route.role + "` — " + .route.rationale, "",
        "**Evidence**",
        (.evidence[] | "- `" + .artifact + "` — " + .observation),
        "",
        (if (.missing_evidence|length) > 0 then "**Could not be checked**" else empty end),
        (.missing_evidence[]? | "- " + .),
        "",
        (if (.next_checks|length) > 0 then "**Next**" else empty end),
        (.next_checks[]? | "- " + .)
      ' "${OUT}/verdict.json"
  else
      jq -r '
        "**Insufficient evidence for an automated conclusion.**", "",
        "Gate: " + .gate.reason, "",
        (if (.missing_evidence|length) > 0 then "**Could not be checked**" else empty end),
        (.missing_evidence[]? | "- " + .),
        "",
        (if (.next_checks|length) > 0 then "**Suggested next steps**" else empty end),
        (.next_checks[]? | "- " + .)
      ' "${OUT}/verdict.json"
  fi
  echo
  echo "_No owner was assigned automatically. Routing above is advisory._"
} > "${OUT}/note.md"

echo -e "${SB_CYAN}verdict${SB_NC}: ${OUT}/verdict.json"
echo -e "${SB_CYAN}note${SB_NC}:    ${OUT}/note.md"
jq -r '"  class=" + .classification.class + " conf=" + (.classification.confidence|tostring) +
       " route=" + .route.role + " publish=" + (.gate.publish|tostring)' "${OUT}/verdict.json"
