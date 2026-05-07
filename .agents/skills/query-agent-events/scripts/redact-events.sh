#!/usr/bin/env bash
# redact-events.sh -- opt-in redaction of identifying fields.
#
# Replaces UUID-shaped values in known identifying fields with
# stable placeholders like <redacted-A>, <redacted-B>, ... so
# patterns are still identifiable (same machine_guid -> same
# placeholder within the dump) but the raw value is gone.
#
# Use this only when sharing a dump externally. Default
# workflow keeps raw events under .local/audits/...
# (gitignored).

set -euo pipefail

usage() {
    cat <<'EOF'
redact-events.sh [--input PATH] [--output PATH]

Redacts identifying fields in a Function-envelope JSON dump or
a flat array of objects.

Identifying fields redacted:
  AE_AGENT_ID, AE_HOST_ID, AE_AGENT_NODE_ID, AE_AGENT_CLAIM_ID,
  AE_HOST_BOOT_ID, AE_AGENT_EPHEMERAL_ID, AE_HW_SYS_UUID,
  _MACHINE_ID, _BOOT_ID

Stable mapping: same value -> same placeholder within the dump.

Defaults:
  --input  stdin
  --output stdout
EOF
}

INPUT=
OUTPUT=
while [ $# -gt 0 ]; do
    case "$1" in
        --input)  INPUT="$2"; shift 2 ;;
        --output) OUTPUT="$2"; shift 2 ;;
        -h|--help) usage; exit 0 ;;
        *) echo "Unknown option: $1" >&2; usage >&2; exit 2 ;;
    esac
done

if [ -n "$INPUT" ]; then exec < "$INPUT"; fi
if [ -n "$OUTPUT" ]; then exec > "$OUTPUT"; fi

jq -c '
def redact_fields:
  ["AE_AGENT_ID","AE_HOST_ID","AE_AGENT_NODE_ID","AE_AGENT_CLAIM_ID",
   "AE_HOST_BOOT_ID","AE_AGENT_EPHEMERAL_ID","AE_HW_SYS_UUID",
   "_MACHINE_ID","_BOOT_ID"];

def alphabet:
  ["A","B","C","D","E","F","G","H","I","J","K","L","M","N","O","P",
   "Q","R","S","T","U","V","W","X","Y","Z"];

. as $root
| reduce (redact_fields[]) as $f (
    {state: {map: {}, idx: 0}, root: $root};
    . as {state: $st, root: $r}
    | ($r | .. | objects | select(has($f)) | .[$f] | tostring) as $vals
    | reduce $vals as $v (
        .;
        if (.state.map | has($v)) then
          .
        else
          .state.map[$v] = ("<redacted-" + (alphabet[(.state.idx % 26)]) + (if .state.idx >= 26 then ((.state.idx / 26 | floor) | tostring) else "" end) + ">")
          | .state.idx += 1
        end
      )
  )
| .root as $r
| .state.map as $m
| ($r | walk(
    if type == "object" then
      with_entries(
        if (.key | IN("AE_AGENT_ID","AE_HOST_ID","AE_AGENT_NODE_ID","AE_AGENT_CLAIM_ID","AE_HOST_BOOT_ID","AE_AGENT_EPHEMERAL_ID","AE_HW_SYS_UUID","_MACHINE_ID","_BOOT_ID"))
        then .value = ($m[(.value | tostring)] // .value)
        else .
        end
      )
    else .
    end
  ))
'
